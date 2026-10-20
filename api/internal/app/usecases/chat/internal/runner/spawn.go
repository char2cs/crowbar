package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	h := rs.acquire(ctx, chatID)
	defer h.release()

	runnerID, err = rs.spawnRunner(ctx, chatID, workspaceID, providerID, spawnInput{}, true)
	if err != nil {
		return "", "", rs.discardSpawnedChat(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

func (rs *Runners) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	h := rs.acquire(ctx, chatID)
	defer h.release()

	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: start runner: chat: %w", err)
	}
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStarting)()
	return rs.spawnRunner(ctx, chatID, chat.WorkspaceID, providerID, spawnInput{}, false)
}

func (rs *Runners) discardSpawnedChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := rs.conversations.PurgeLocked(ctx, chatID); err != nil && !errors.Is(err, agentchat.ErrNotFound) {
		slog.WarnContext(ctx, "agent: discard chat of a refused spawn (best-effort, reporting the spawn failure)",
			"chat_id", chatID, "err", err)
	}
	return cause
}

//nolint:funlen // orchestrates preflight, path resolution, attachment materialization, descriptor render, spawn-plan build and fork in one strict sequence; splitting would scatter the abort-on-failure cleanup this function is responsible for at each step
func (rs *Runners) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	in spawnInput,
	create bool,
) (_ string, err error) {
	runnerID := newRunnerID(in.runnerID)
	defer rs.recordSpawnFailure(ctx, chatID, runnerID, create, &err)
	pre, err := rs.spawnPreflight(ctx, chatID, providerID, create)
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
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}
	launch, sessionSteps, err := rs.spawnLaunch(ctx, chatID, descriptor, in.launch)
	if err != nil {
		return "", err
	}

	// Copies of promptMessage and conversation for dispatch — the durable ledger
	// text is never mutated. conversation (AssembleConversation's rendering of
	// the prior exchange, handed to a freshly spawned CLI on a restart or a
	// provider switch) carries whatever attachment references the ORIGINAL
	// turns held, exactly like promptMessage does for the live one — without
	// this, only the CURRENT prompt's attachments resolved to real paths, and
	// every earlier attachment a resumed/switched-to CLI was handed the
	// literal logical reference for a file it therefore could not read.
	//
	// The sigil guard runs BEFORE materialization, while the attachment is still
	// the logical `chats/<id>/attachments/<file>` reference its pattern is
	// written against; the escape it may prepend does not disturb the rewrite.
	sigils, escape := descriptor.PromptLeadingSigils()
	guardedMessage := promptsigil.Guard(sigils, escape, chatID, in.promptMessage)
	dispatchMessage := materializeAttachmentsForDispatch(paths.chatsDir, chatID, guardedMessage)
	dispatchConversation := materializeAttachmentsForDispatch(paths.chatsDir, chatID, in.conversation)

	// The tool surface is switched off by rendering a descriptor that does not
	// declare one, rather than by filtering steps at the injection site: WHERE those
	// steps land is the descriptor's business (a variadic flag may need its pair
	// immediately behind it), and this function has no business knowing that.
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

	// Register the injected document BEFORE the CLI can run: a provider whose only
	// resume channel is a user message fires its user-prompt hook with this
	// exact text the moment it starts, and that echo must never be recorded as a
	// ledger turn — that is what made handoffs nest inside themselves.
	//
	// Only when something was ACTUALLY injected. contextInject is the sole channel for
	// both the document and the pointer, so an un-injected spawn has nothing that can
	// echo — and since the capability preamble makes tctx.Context non-empty on every
	// spawn, registering unconditionally would leave a guard behind for text no CLI
	// was ever given.
	//
	// Unconditional otherwise — including when promptMessage is also set: a real
	// prompt can ride the SAME positional as the injected document
	// (mergeLeadingPositional), and the hook side (ConsumeInjectedPrefix,
	// turn.go) is what tells "bare echo" and "echo with a real prompt merged
	// ahead of it" apart, returning the remainder in the second case rather
	// than swallowing the whole turn. Registering only for the bare case (a
	// prior version of this gate) left the merged case with nothing
	// registered at all — the injected preamble was then recorded verbatim as
	// what the user typed, corrupting the ledger, the derived title, and
	// every hash-based "was this accepted" check downstream.
	if inject {
		rs.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}

	// The chat's one entry, claimed before any process exists: a second
	// spawn is refused here, never after it has forked. Whatever the process
	// reports before runner_started commits waits on the chat's lock, held here.
	if err := rs.live.add(engineagents.Runner{
		ID: runnerID, WorkspaceID: workspaceID, ProviderID: providerID, CurrentChatID: chatID,
	}); err != nil {
		rs.agents.ForgetRunner(runnerID)
		return "", err
	}
	// The surface this process lands on, recorded before anything can ask
	// (ShowingNativeView reads it).
	rs.surfaces.set(runnerID, surfaceForSpawn(descriptor, pre.surface))
	// The api connection comes up (or not) BEFORE the PTY plan is rendered:
	// when it does, the runner is that connection and the plan never runs;
	// when it does not, the session runs over the PTY's hooks alone.
	attachArgv := rs.apiTransportForSurface(
		ctx, runnerID, providerID, descriptor, tctx, resumeContextFor(in.resuming, inject, tctx), pre.surface,
	)
	steps := buildSpawnSteps(
		descriptor, in.resuming, inject, sel, sessionSteps, in.finalSteps,
	)

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		rs.abandonUnforked(runnerID)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}
	pointPlanAtAttach(plan, attachArgv)
	if err := rs.bindAssigned(ctx, chatID, launch); err != nil {
		rs.abandonUnforked(runnerID)
		return "", err
	}

	// binpath.Resolve, never the bare descriptor cmd: the PTY exec's argv[0] through
	// exec.Command, which resolves a bare name against the DAEMON's PATH — plan.Env is
	// ignored for the lookup. A launchd-started .app daemon has a minimal PATH that
	// misses ~/.local/bin, where vendor CLIs install, so a bare name made every
	// spawn die with "executable file not found in $PATH". An unresolvable cmd passes
	// through unchanged, preserving that error for a CLI that genuinely is not installed.
	termSessID, carried, err := rs.forkOrAdopt(ctx, forkRequest{
		runnerID:    runnerID,
		providerID:  providerID,
		chatID:      chatID,
		worktree:    worktree,
		crowbarHome: crowbarHome,
		tmpDir:      tmpDir,
		argv:        append([]string{plan.Executable}, plan.Argv...),
		env:         plan.Env,
		// The SAME text the descriptor's prompt_submit steps just rendered into
		// plan.Argv — never the raw ledger text: attachments are materialized
		// and the leading-sigil escape applied above, and the carrier that ends
		// up delivering it must send exactly what the argv would have.
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
	}, launch.resumeID, create); err != nil {
		if !errors.Is(err, ErrProviderExitedDuringStartup) {
			rs.agents.ForgetRunner(runnerID)
		}
		return "", err
	}
	return runnerID, nil
}

// abandonUnforked forgets what a spawn set up before it forked anything.
func (rs *Runners) abandonUnforked(runnerID string) {
	rs.apiConns.drop(runnerID)
	rs.agents.ForgetRunner(runnerID)
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
	rs.agents.ForgetRunner(req.runnerID)
	reapRunnerDir(ctx, req.crowbarHome, req.tmpDir)

	// A CLI that is not installed is the ONE spawn failure the user can act on, so
	// it travels as its own sentinel (→ 424, a named message in the UI) rather than
	// being buried in a wrap chain that maps to an opaque 500. The provider id, not
	// the resolved argv[0], is what the UI can name.
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
	// surface is the chat's own stored landing VIEW — see storedSurface.
	surface string
}

func (rs *Runners) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	create bool,
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
	threads, err := rs.conversations.ThreadContext(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	// The selection is also what gets RECORDED on the runner, so the record and the
	// argv are rendered from ONE read: two reads could disagree across a concurrent
	// change and leave a process whose recorded selection is not the one it runs.
	sel, err := rs.conversations.ChatSelection(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	surface, err := rs.storedSurface(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	return spawnPreflight{mcpOn: mcpOn, threads: threads, selection: sel, surface: surface}, nil
}

type forkRequest struct {
	runnerID   string
	providerID string
	// chatID is the terminal engine's session-scoping key — NOT the workspace id
	// this used to carry: a chat with no worktree of its own has an empty one, so
	// every such runner registered under the key "". worktree below is the CWD.
	chatID      string
	worktree    string
	crowbarHome string
	tmpDir      string
	argv        []string
	env         []string
	// promptMessage is the user text this spawn exists to deliver, "" when it
	// carries none. It is here, and not only inside argv, because argv is not
	// always a carrier: forkOrAdopt's adopt branch forks no process at all, so
	// whichever branch runs has to be able to SEE that it owes a delivery.
	// See carryPromptOverAPIConn (apirunner.go) for the invariant.
	promptMessage string
	// argvSelection/apiSelection are the SAME invariant for the model/effort
	// choice: the part of it each carrier renders. The branch that runs
	// returns its own (forkOrAdopt), so recordRunner cannot stamp a selection
	// the process never received — see carriedSelection (apirunner.go).
	argvSelection engineagents.Selection
	apiSelection  engineagents.Selection
	// resume is how this process's exit tells a refusal of its session.
	resume resumeWatch
}

func (rs *Runners) teardownAfterPersistFailure(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := rs.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown after persist failure",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	// applyAPITransport runs before this point in spawnRunner and can have
	// already established a live serve process by the time persistence fails —
	// that process has no PTY of its own to fall with the one just terminated
	// above (see quitOutgoingCLI's comment), so it leaks unless dropped here too.
	rs.apiConns.drop(runnerID)
	return cause
}

func (rs *Runners) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}
