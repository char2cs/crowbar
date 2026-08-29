package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (rs *Runners) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	defer rs.spawns.Lock(chatID)()

	runnerID, err = rs.spawnRunner(ctx, chatID, workspaceID, providerID, "", nil, nil, "", 0, false, "", true, "")
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
	defer rs.spawns.Lock(chatID)()

	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: start runner: chat: %w", err)
	}
	return rs.spawnRunner(ctx, chatID, chat.WorkspaceID, providerID, "", nil, nil, "", 0, false, "", false, "")
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

func (rs *Runners) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	preallocatedRunnerID string,
	extraSteps []engineagents.InjectStep,
	finalSteps []engineagents.InjectStep,
	conversation string,
	gapTurns int,
	resuming bool,
	launchSessionID string,
	create bool,
	promptMessage string,
) (string, error) {
	pre, err := rs.spawnPreflight(ctx, chatID, providerID, create)
	if err != nil {
		return "", err
	}
	threads, sel := pre.threads, pre.selection
	runnerID := newRunnerID(preallocatedRunnerID)

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
	// The tool surface is switched off by rendering a descriptor that does not
	// declare one, rather than by filtering steps at the injection site: WHERE those
	// steps land is the descriptor's business (claude's --mcp-config is variadic and
	// needs the --settings pair immediately behind it), and this function has no
	// business knowing that.
	descriptor = descriptor.WithTools(pre.mcpOn)

	// Resolved against THIS provider, never the chat's own raw stored intent
	// directly: a level the provider does not declare is not offered for it
	// at all (an explicit SetChatPermissionLevel already refuses one), but
	// the chat's own durable choice — the global default it was seeded
	// with, or an earlier provider's own valid pick — can still name one
	// this provider cannot reach, and a spawn must run under SOMETHING
	// rather than fail. Never written back: the chat's own stored intent is
	// untouched, so switching providers again resolves fresh each time.
	sel.PermissionLevel = resolvePermissionLevel(descriptor.PermissionLevels(), sel.PermissionLevel)

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
		launchSessionID: launchSessionID,
		threads:         threads,
		conversation:    conversation,
		promptMessage:   promptMessage,
		gapTurns:        gapTurns,
		resuming:        resuming,
		selection:       sel,
		permissionVars:  descriptor.PermissionVars(sel.PermissionLevel),
	})

	steps := buildSpawnSteps(descriptor, resuming, inject, sel, extraSteps, finalSteps)

	// Register the injected document BEFORE the CLI can run: a provider whose only
	// resume channel is a user message (codex) fires its user-prompt hook with this
	// exact text the moment it starts, and that echo must never be recorded as a
	// ledger turn — that is what made handoffs nest inside themselves.
	//
	// Only when something was ACTUALLY injected. contextInject is the sole channel for
	// both the document and the pointer, so an un-injected spawn has nothing that can
	// echo — and since the capability preamble makes tctx.Context non-empty on every
	// spawn, registering unconditionally would leave a guard behind for text no CLI
	// was ever given.
	if inject {
		rs.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		// The injected-context entry above was registered before the CLI could exist, and
		// only reconcileRunnerExit (via the onExit callback) ever forgets it — a callback
		// that never fires when the CLI never goes live. Forget it here, or every failed
		// spawn leaks one handoff-sized string until the daemon restarts.
		rs.agents.ForgetRunner(runnerID)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}
	rs.applyAPITransport(ctx, runnerID, providerID, descriptor, tctx, plan, resumeContextFor(resuming, inject, tctx))

	// binpath.Resolve, never the bare descriptor cmd: the PTY exec's argv[0] through
	// exec.Command, which resolves a bare name against the DAEMON's PATH — plan.Env is
	// ignored for the lookup. A launchd-started .app daemon has a minimal PATH that
	// misses ~/.local/bin, where claude and codex install, so a bare name made every
	// spawn die with "executable file not found in $PATH". An unresolvable cmd passes
	// through unchanged, preserving that error for a CLI that genuinely is not installed.
	argv := append([]string{plan.Executable}, plan.Argv...)
	termSessID, err := rs.forkCLI(ctx, forkRequest{
		runnerID:    runnerID,
		providerID:  providerID,
		workspaceID: workspaceID,
		worktree:    worktree,
		crowbarHome: crowbarHome,
		tmpDir:      tmpDir,
		argv:        argv,
		env:         plan.Env,
	})
	if err != nil {
		return "", err
	}

	if err := rs.recordRunner(
		ctx, chatID, workspaceID, providerID, runnerID, termSessID, launchSessionID, sel, create,
	); err != nil {
		rs.pendingHooks.Discard(runnerID)
		rs.agents.ForgetRunner(runnerID)
		return "", err
	}
	// Keep the barrier installed throughout replay. A hook arriving while an
	// earlier buffered hook is being applied joins the next batch, so it cannot
	// overtake session_start or user_prompt on the normal persisted-runner path.
	//
	// exitedDuringStartup means exactly one thing, and it is narrower than its name:
	// the PTY died BEFORE the runner row committed, so the exit callback had no row
	// to reconcile against and left the fact here instead. It is a RACE the CLI has
	// to lose to be caught — one that dies 50ms later wins it, gets a 201, and
	// reconciles through onRunnerExit into an ordinary DORMANT chat.
	//
	// So 424 is not a guarantee that a chat handed back has a living CLI behind it,
	// and nothing may be built on reading it that way. Both outcomes are honest and
	// both are visible — a refusal that names the dependency, or a chat the panel
	// already draws as dormant with no runner on it — and the only way to collapse
	// them into one deterministic answer would be to wait or probe after EVERY
	// spawn, paying latency on the common path to close a corner. That is the trade,
	// deliberately made in this direction; it is not an oversight to be tightened.
	exitedDuringStartup := rs.pendingHooks.Finish(runnerID, func(hook inflight.Hook) {
		rs.turns.ReplayStartupHook(runnerID, hook)
	})
	if exitedDuringStartup {
		// onExit could not reconcile before the row existed. Now it does, after
		// every hook the provider emitted before dying has had its ordered chance
		// to update the ledger and prompt journal.
		rs.reconcileRunnerExit(context.Background(), runnerID)
		return "", ErrProviderExitedDuringStartup
	}
	return runnerID, nil
}

func (rs *Runners) forkCLI(
	ctx context.Context,
	req forkRequest,
) (string, error) {
	if err := rs.pendingHooks.Register(req.runnerID); err != nil {
		rs.agents.ForgetRunner(req.runnerID)
		worktreepath.RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)
		return "", fmt.Errorf("agent: spawn runner: install hook startup barrier: %w", err)
	}
	termSessID, err := rs.term.CreateCommand(ctx, req.workspaceID, req.worktree, req.argv, req.env,
		rs.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir))
	if err == nil {
		return termSessID, nil
	}
	rs.pendingHooks.Discard(req.runnerID)
	rs.agents.ForgetRunner(req.runnerID)
	worktreepath.RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)

	// A CLI that is not installed is the ONE spawn failure the user can act on, so
	// it travels as its own sentinel (→ 424, a named message in the UI) rather than
	// being buried in a wrap chain that maps to an opaque 500. The provider id, not
	// the resolved argv[0], is what the UI can name.
	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return "", fmt.Errorf("%w: %s", engineterminal.ErrCommandNotFound, req.providerID)
	}
	return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
}

func (rs *Runners) recordRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	runnerID string,
	termSessID string,
	launchSessionID string,
	sel engineagents.Selection,
	create bool,
) error {
	now := time.Now()
	if create {
		created, err := rs.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: workspaceID,
			Type:        domain.ChatTypeChat,
			Now:         now,
		})
		if err != nil {
			return rs.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
		rs.work.Set(chatID, created.Working)
		rs.seedPermissionLevel(ctx, chatID)
	}
	if _, err := rs.runnerStore.Start(ctx, agentrunner.StartInput{
		RunnerID:        runnerID,
		WorkspaceID:     workspaceID,
		ProviderID:      providerID,
		TerminalSession: termSessID,
		ChatID:          chatID,
		LaunchSessionID: launchSessionID,
		// The selection this process was ACTUALLY launched with, recorded from the
		// same read that rendered its argv. It is the only authority on what this
		// CLI is running: nothing can ask the process later.
		LaunchModel:  sel.Model,
		LaunchEffort: sel.Effort,
		// The RESOLVED level — after resolvePermissionLevel's clamp, not the
		// chat's own raw stored intent — because this is a record of what
		// actually got launched, the same fact LaunchModel/LaunchEffort are.
		LaunchPermissionLevel: sel.PermissionLevel,
		Now:                   now,
	}); err != nil {
		return rs.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
			fmt.Errorf("agent: spawn runner: start runner: %w", err))
	}

	// A Start is a PLACEMENT, so it obeys the same rule a Move does: whoever else is on this
	// chat is retired. The spawn gate cannot cover this, and it is not a hairline window —
	// it is as wide as a process fork:
	//
	//	a gated SwitchProvider quits and DISPLACES the outgoing CLI, leaving the chat with
	//	nobody on it; a HOOK (never gated, and never may be) moves another live CLI onto it,
	//	evicting nobody because nobody is there; and only THEN do we resolve a descriptor,
	//	render a tmp dir, fork a process and land here.
	//
	// Without this the chat ends up holding both, indefinitely — and the loser is INVISIBLE,
	// because the serving read hands out the newest arrival while the other one goes on
	// appending to the chat's ledger. Start is SendWait, so this read already sees us.
	rs.retireOthersOn(ctx, chatID, runnerID)
	return nil
}

func (rs *Runners) mintRunnerToken(runnerID string) string {
	if rs.minter == nil {
		return ""
	}
	return rs.minter.Mint(runnerID)
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

	// launchSessionID is the prior session/thread id to resume, if any — the
	// SAME value hooks-transport's own resume argv already carries, and what
	// an api-transport connection's EstablishSession needs to know whether to
	// run Fresh (nothing known yet) or Resume (known, but not yet loaded on
	// THIS connection).
	launchSessionID string
}

type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
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
	return spawnPreflight{mcpOn: mcpOn, threads: threads, selection: sel}, nil
}

type forkRequest struct {
	runnerID    string
	providerID  string
	workspaceID string
	worktree    string
	crowbarHome string
	tmpDir      string
	argv        []string
	env         []string
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

func (rs *Runners) onRunnerExit(home, runnerID, tmpDir string) func() {
	return func() {
		worktreepath.RemoveUnderHome(context.Background(), home, tmpDir)
		// A dead PTY takes its api-transport connection (serve process + driver)
		// with it — never leaked, and safe to call for a hooks-only runner that
		// never had one.
		rs.apiConns.drop(runnerID)
		// CreateCommand can observe process exit before recordRunner has a
		// terminal-session id to persist. The startup barrier remembers that fact;
		// spawnRunner reconciles it immediately after persistence and ordered hook
		// replay, instead of this callback missing the not-yet-existing row forever.
		if rs.pendingHooks.MarkExited(runnerID) {
			return
		}
		rs.reconcileRunnerExit(context.Background(), runnerID)
	}
}

// buildSpawnSteps assembles the ordered InjectStep list a spawn's SpawnPlan
// renders against: extraSteps first, then the descriptor's own selection and
// context steps, then finalSteps — positional user prompts are final by
// contract (Claude's variadic --mcp-config must already be terminated by
// later options, and codex's resume subcommand/id must precede the message).
//
// descriptor.SelectionSteps contributes an EMPTY slice for a chat with no
// model/effort choice, or a provider declaring no such block — so this costs
// nothing on a spawn not using the feature, and the argv is byte-identical to
// one rendered before it existed.
func buildSpawnSteps(
	descriptor engineagents.Agent,
	resuming, inject bool,
	sel engineagents.Selection,
	extraSteps, finalSteps []engineagents.InjectStep,
) []engineagents.InjectStep {
	steps := append([]engineagents.InjectStep{}, extraSteps...)
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if contextStepsAllowed(resuming, inject, descriptor) {
		steps = append(steps, descriptor.ContextSteps(resuming)...)
	}
	return append(steps, finalSteps...)
}

// contextStepsAllowed is whether ContextSteps — a CLI argv, a POSITIONAL
// PROMPT on the resume path — may be rendered at all. False exactly when the
// redundant hooks-only PTY this same spawn's applyAPITransport call has
// already resumed over the api connection would otherwise answer it as its
// own genuine first turn (a provider whose only resume channel is a user
// message, e.g. codex — see apiOwnsResume). Never suppressed for a FRESH
// inject: an unresumed spawn's ContextSteps is silent config, nothing for the
// PTY to act on. resumeContextFor, just below, is this same routing decision
// for the OTHER channel — InjectAt over the api connection itself.
func contextStepsAllowed(resuming, inject bool, descriptor engineagents.Agent) bool {
	return inject && (!resuming || !apiOwnsResume(descriptor))
}

// resumeContextFor is the gap document a resumed api-transport connection's
// applyAPITransport call hands to InjectAt("context") — see that function's own
// comment for why this rides a separate channel from EstablishSession's own
// "context" value. Empty whenever inject's own gate says there is nothing to
// hand over, or this spawn isn't a resume at all: a fresh establish already
// carries tctx.Context as thread/start's developerInstructions.
func resumeContextFor(resuming, inject bool, tctx engineagents.TemplateCtx) string {
	if resuming && inject {
		return tctx.Context
	}
	return ""
}

func (rs *Runners) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}
