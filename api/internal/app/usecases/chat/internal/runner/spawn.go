package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) StartRunner(ctx context.Context, chatID, providerID string) (string, error) {
	return rs.underChat(ctx, chatID, "start runner", func(chat domain.Chat) (string, error) {
		return rs.startLocked(ctx, chat, providerID)
	})
}

// OpenChat is a just-created chat's launch. Only a chat landing on its terminal
// gets its CLI now, that view being nothing without one; any other stays dormant
// ("") until its first prompt, whose launch carries it: one CLI, not two.
func (rs *Runners) OpenChat(ctx context.Context, chatID, providerID string) (string, error) {
	return rs.underChat(ctx, chatID, "open chat", func(chat domain.Chat) (string, error) {
		return rs.openLocked(ctx, chat, providerID)
	})
}

// underChat runs launch on chatID's current row with its lock held.
func (rs *Runners) underChat(
	ctx context.Context, chatID, op string, launch func(domain.Chat) (string, error),
) (string, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: %s: %w", op, err)
	}
	defer h.release()
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: %s: chat: %w", op, err)
	}
	return launch(chat)
}

// openLocked refuses what a launch would refuse, so no chat is made that no
// prompt could start, then launches only a chat that lands on its terminal.
func (rs *Runners) openLocked(ctx context.Context, chat domain.Chat, providerID string) (string, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return "", err
	}
	descriptor, _, err := rs.promptAgent(ctx, chat, providerID)
	if err != nil {
		return "", err
	}
	if surfaceForSpawn(descriptor, chat.Surface) != engineagents.SurfaceTerminal {
		return "", nil
	}
	return rs.startLocked(ctx, chat, providerID)
}

func (rs *Runners) startLocked(ctx context.Context, chat domain.Chat, providerID string) (string, error) {
	defer rs.enterPhase(ctx, chat.ID, snapshot.PhaseStarting)()
	return rs.spawnRunner(ctx, chat.ID, chat.WorkspaceID, providerID, spawnInput{})
}

//nolint:funlen // orchestrates preflight, path resolution, attachment materialization, descriptor render, spawn-plan build and fork in one strict sequence; splitting would scatter the abort-on-failure cleanup this function is responsible for at each step
func (rs *Runners) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	in spawnInput,
) (_ string, err error) {
	runnerID := newRunnerID(in.runnerID)
	defer rs.recordSpawnFailure(ctx, chatID, runnerID, &err)
	pre, err := rs.spawnPreflight(ctx, chatID, providerID, in.selection)
	if err != nil {
		return "", err
	}
	threads, sel := pre.threads, pre.selection

	paths, err := rs.spawnPaths(ctx, chatID, workspaceID, runnerID, providerID)
	if err != nil {
		return "", err
	}
	crowbarHome, projectID, repoID := paths.crowbarHome, paths.projectID, paths.repoID
	worktree, tmpDir := paths.worktree, paths.tmpDir

	descriptor, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}
	launch, sessionSteps, err := rs.spawnLaunch(ctx, chatID, descriptor, in.launch)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}

	// Dispatch copies of the prompt and the prior conversation, both with attachment
	// refs materialized (the ledger text is never mutated). The sigil guard runs first,
	// while refs are still the logical form its pattern is written against.
	sigils, escape := descriptor.PromptLeadingSigils()
	guardedMessage := promptsigil.Guard(sigils, escape, chatID, in.promptMessage)
	dispatchMessage := materializeAttachmentsForDispatch(paths.chatsDir, chatID, guardedMessage)
	dispatchConversation := materializeAttachmentsForDispatch(paths.chatsDir, chatID, in.conversation)

	// Tools are switched off by rendering a descriptor without them, not by filtering
	// steps here: where those steps land (flag pairing) is the descriptor's business.
	descriptor = descriptor.WithTools(pre.mcpOn)

	// Never the chat's own raw stored intent directly — see
	// resolveSelectionForSpawn (selection.go) for why.
	sel = resolveSelectionForSpawn(descriptor, sel)

	tctx, inject := rs.renderSpawnContext(spawnContext{
		chatID:          chatID,
		workspaceID:     workspaceID,
		providerID:      providerID,
		projectID:       projectID,
		repoID:          repoID,
		runnerID:        runnerID,
		tmpDir:          tmpDir,
		worktree:        worktree,
		crowbarHome:     crowbarHome,
		launchSessionID: launch.resumeID,
		threads:         threads,
		conversation:    dispatchConversation,
		promptMessage:   dispatchMessage,
		gapTurns:        in.gapTurns,
		resuming:        in.resuming,
		selection:       sel,
		permissionVars:  descriptor.PermissionVars(sel.PermissionLevel),
	})

	// Register an ACTUALLY injected document before the CLI runs, so its echo through
	// the user-prompt hook is never recorded as a turn — also when a real prompt is merged
	// into the same positional: ConsumeInjectedPrefix strips the echo, keeps the rest.
	if inject {
		rs.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}

	// The chat's one entry, claimed before any process exists: a second
	// spawn is refused here, never after it has forked. Whatever the process
	// reports before runner_started commits waits on the chat's lock, held here.
	if err := rs.live.add(engineagents.Runner{
		ID: runnerID, WorkspaceID: workspaceID, ProviderID: providerID, CurrentChatID: chatID,
	}); err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}
	// The surface this process lands on, recorded before anything can ask
	// (ShowingNativeView reads it).
	rs.surfaces.set(runnerID, surfaceForSpawn(descriptor, pre.surface))
	// The api connection comes up (or not) BEFORE the PTY plan is rendered:
	// when it does, the runner is that connection and the plan never runs;
	// when it does not, the session runs over the PTY's hooks alone.
	attachArgv, refused := rs.apiTransportForSurface(
		ctx, runnerID, providerID, descriptor, tctx, resumeContextFor(in.resuming, inject, tctx), pre.surface,
	)
	if refused != nil {
		// The plan resumes the session the provider just refused.
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", fmt.Errorf("%w: %w", ErrSpawnSessionRefused, refused)
	}
	steps := buildSpawnSteps(
		descriptor, in.resuming, inject, sel, sessionSteps, in.finalSteps,
	)

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}
	pointPlanAtAttach(plan, attachArgv)
	if err := rs.bindAssigned(ctx, chatID, launch); err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}

	// binpath.Resolve, never the bare cmd: exec resolves argv[0] against the DAEMON's
	// PATH, which under launchd misses ~/.local/bin. An unresolvable cmd passes through,
	// keeping the not-found error for a CLI that is genuinely missing.
	termSessID, carried, err := rs.forkOrAdopt(ctx, forkRequest{
		runnerID:    runnerID,
		providerID:  providerID,
		chatID:      chatID,
		worktree:    worktree,
		crowbarHome: crowbarHome,
		tmpDir:      tmpDir,
		argv:        append([]string{plan.Executable}, plan.Argv...),
		env:         plan.Env,
		// The SAME text prompt_submit rendered into plan.Argv, never the raw ledger
		// text, so whichever carrier delivers sends exactly what the argv would have.
		promptMessage: dispatchMessage,
		// What each carrier would take, computed before either runs;
		// forkOrAdopt returns whichever one actually did.
		argvSelection: carriedSelection(sel, descriptor.SelectionSteps),
		apiSelection:  carriedSelection(sel, descriptor.SelectionAPISteps),
		resume:        resumeWatch{chatID: chatID, providerID: providerID, sessionID: launch.resumeID, agent: descriptor},
	}, attachArgv)
	if err != nil {
		return "", err
	}

	if err := rs.recordRunner(ctx, engineagents.Runner{
		ID: runnerID, WorkspaceID: workspaceID, ProviderID: providerID, TerminalSession: termSessID,
		CurrentChatID: chatID, LaunchModel: carried.Model, LaunchEffort: carried.Effort,
		LaunchPermissionLevel: carried.PermissionLevel,
	}, launch.resumeID, in.selection); err != nil {
		if !errors.Is(err, ErrProviderExitedDuringStartup) {
			rs.agents.ForgetRunner(runnerID)
		}
		return "", err
	}
	return runnerID, nil
}

// abandonUnforked forgets what a spawn set up before it forked anything: no
// exit callback exists yet to reap the runner's directory or surface entry.
func (rs *Runners) abandonUnforked(ctx context.Context, runnerID string, paths spawnPaths) {
	rs.apiConns.drop(runnerID)
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	reapRunnerDir(ctx, paths.crowbarHome, paths.tmpDir)
}

func (rs *Runners) forkCLI(
	ctx context.Context,
	req forkRequest,
) (string, error) {
	//nolint:contextcheck // the exit outlives the request; its values must not reach it.
	termSessID, err := rs.term.CreateCommand(ctx, req.chatID, req.worktree, req.argv, req.env,
		rs.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir, req.resume))
	if err == nil {
		return termSessID, nil
	}
	rs.abandonUnforked(ctx, req.runnerID, req.paths())

	// A missing CLI is the ONE spawn failure the user can act on, so it keeps its own
	// sentinel (424) naming the provider id rather than an opaque wrapped 500.
	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return "", fmt.Errorf("%w: %s", engineterminal.ErrCommandNotFound, req.providerID)
	}
	return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
}

func newRunnerID(
	preallocated string,
) string {
	if preallocated != "" {
		return preallocated
	}
	return uuid.NewString()
}

type spawnContext struct {
	chatID        string
	workspaceID   string
	providerID    string
	projectID     string
	repoID        string
	runnerID      string
	tmpDir        string
	worktree      string
	crowbarHome   string
	threads       string
	conversation  string
	promptMessage string
	gapTurns      int
	resuming      bool
	selection     engineagents.Selection
	// permissionVars is the resolved permission level's own named values
	// (see spec.PermissionLevelSpec's own doc comment) — nil for a provider
	// that declares none.
	permissionVars map[string]string

	// launchSessionID is the bound session to resume, if any — the SAME id the
	// resume argv carries, and what tells an api connection's EstablishSession
	// to run Resume rather than Fresh.
	launchSessionID string
}

type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
	// surface is the chat's own stored landing VIEW.
	surface string
}

func (rs *Runners) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	staged *domain.ChatSelection,
) (spawnPreflight, error) {
	// This is the ONE seam every vendor CLI is launched through, which makes it the
	// only place a disabled provider can actually be stopped.
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return spawnPreflight{}, err
	}
	// The tool switch is a SEPARATE axis from whether the provider is enabled: a CLI
	// spawned with its tools off still comes up, still fires its hooks and still
	// holds a normal chat.
	mcpOn, err := rs.providers.ProviderMCPEnabled(ctx, providerID)
	if err != nil {
		return spawnPreflight{}, err
	}
	threads, err := rs.conversations.ThreadContext(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, err
	}
	// The selection is also what gets RECORDED on the runner, so the record and the
	// argv are rendered from ONE read: two reads could disagree across a concurrent
	// change and leave a process whose recorded selection is not the one it runs.
	sel, err := rs.conversations.ChatSelection(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, err
	}
	if staged != nil {
		sel.Model, sel.Effort = staged.Model, staged.Effort
	}
	// Read at every spawn, so each one rebuilds on the chat's durable landing VIEW.
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, fmt.Errorf("agent: spawn runner: chat surface: %w", err)
	}
	return spawnPreflight{mcpOn: mcpOn, threads: threads, selection: sel, surface: chat.Surface}, nil
}

type forkRequest struct {
	runnerID   string
	providerID string
	// chatID is the terminal engine's session-scoping key, not the workspace id (empty
	// for a chat with no worktree of its own). worktree below is the CWD.
	chatID      string
	worktree    string
	crowbarHome string
	tmpDir      string
	argv        []string
	env         []string
	// promptMessage is the user text this spawn must deliver ("" for none). It is not
	// only in argv because the adopt branch forks nothing, and must still see it owes one.
	promptMessage string
	// argvSelection/apiSelection are what each carrier renders of the selection; the
	// branch that runs returns its own, so recordRunner never stamps one not received.
	argvSelection engineagents.Selection
	apiSelection  engineagents.Selection
	// resume is how this process's exit tells a refusal of its session.
	resume resumeWatch
}

// paths are the runner's own paths a failed spawn reaps.
func (req forkRequest) paths() spawnPaths {
	return spawnPaths{crowbarHome: req.crowbarHome, tmpDir: req.tmpDir}
}

// teardownForked ends the CLI a spawn forked before it failed, and returns cause.
func (rs *Runners) teardownForked(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := rs.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown of a failed spawn",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	// The serve process applyAPITransport started has no PTY of its own to
	// fall with the one just terminated, so it leaks unless dropped here too.
	rs.apiConns.drop(runnerID)
	return cause
}
