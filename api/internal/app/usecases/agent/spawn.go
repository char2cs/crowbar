package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/config"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func (u *runnerUsecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	defer u.spawns.lock(chatID)()

	runnerID, err = u.spawnRunner(ctx, chatID, workspaceID, providerID, "", nil, nil, "", 0, false, "", true, "")
	if err != nil {
		return "", "", u.discardSpawnedChat(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

func (u *runnerUsecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: start runner: chat: %w", err)
	}
	return u.spawnRunner(ctx, chatID, chat.WorkspaceID, providerID, "", nil, nil, "", 0, false, "", false, "")
}

func (u *runnerUsecase) discardSpawnedChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.chat.purgeChatLocked(ctx, chatID); err != nil && !errors.Is(err, agentchat.ErrNotFound) {
		slog.WarnContext(ctx, "agent: discard chat of a refused spawn (best-effort, reporting the spawn failure)",
			"chat_id", chatID, "err", err)
	}
	return cause
}

func (u *runnerUsecase) spawnRunner(
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
	pre, err := u.spawnPreflight(ctx, chatID, providerID, create)
	if err != nil {
		return "", err
	}
	threads, sel := pre.threads, pre.selection
	runnerID := newRunnerID(preallocatedRunnerID)

	paths, err := u.spawnPaths(ctx, workspaceID, runnerID, providerID)
	if err != nil {
		return "", err
	}
	crowbarHome, projectID, repoID := paths.crowbarHome, paths.projectID, paths.repoID
	worktree, tmpDir := paths.worktree, paths.tmpDir

	descriptor, err := u.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}
	// The tool surface is switched off by rendering a descriptor that does not
	// declare one, rather than by filtering steps at the injection site: WHERE those
	// steps land is the descriptor's business (claude's --mcp-config is variadic and
	// needs the --settings pair immediately behind it), and this function has no
	// business knowing that.
	descriptor = descriptor.WithTools(pre.mcpOn)

	tctx, inject := u.renderSpawnContext(spawnContext{
		chatID:        chatID,
		workspaceID:   workspaceID,
		providerID:    providerID,
		projectID:     projectID,
		repoID:        repoID,
		runnerID:      runnerID,
		tmpDir:        tmpDir,
		worktree:      worktree,
		crowbarHome:   crowbarHome,
		threads:       threads,
		conversation:  conversation,
		promptMessage: promptMessage,
		gapTurns:      gapTurns,
		resuming:      resuming,
		selection:     sel,
	})

	steps := append([]engineagents.InjectStep{}, extraSteps...)
	// The chat's model/effort choice, rendered by the descriptor that declares it.
	// An unset choice, or a provider declaring no such block, contributes an EMPTY
	// slice — so this line is the whole of the feature's cost on a spawn that is
	// not using it, and the argv is byte-identical to one rendered before it
	// existed.
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if inject {
		steps = append(steps, descriptor.ContextSteps(resuming)...)
	}
	// Positional user prompts are final by contract. In particular Claude's
	// variadic --mcp-config must already have been terminated by later options,
	// and Codex's resume subcommand/id must precede the message.
	steps = append(steps, finalSteps...)

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
		u.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		// The injected-context entry above was registered before the CLI could exist, and
		// only reconcileRunnerExit (via the onExit callback) ever forgets it — a callback
		// that never fires when the CLI never goes live. Forget it here, or every failed
		// spawn leaks one handoff-sized string until the daemon restarts.
		u.agents.ForgetRunner(runnerID)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}

	// binpath.Resolve, never the bare descriptor cmd: the PTY exec's argv[0] through
	// exec.Command, which resolves a bare name against the DAEMON's PATH — plan.Env is
	// ignored for the lookup. A launchd-started .app daemon has a minimal PATH that
	// misses ~/.local/bin, where claude and codex install, so a bare name made every
	// spawn die with "executable file not found in $PATH". An unresolvable cmd passes
	// through unchanged, preserving that error for a CLI that genuinely is not installed.
	argv := append([]string{plan.Executable}, plan.Argv...)
	termSessID, err := u.forkCLI(ctx, forkRequest{
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

	if err := u.recordRunner(
		ctx, chatID, workspaceID, providerID, runnerID, termSessID, launchSessionID, sel, create,
	); err != nil {
		u.pendingHooks.discard(runnerID)
		u.agents.ForgetRunner(runnerID)
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
	exitedDuringStartup := u.pendingHooks.finish(runnerID, func(hook pendingRunnerHook) {
		u.turn.replayStartupHook(runnerID, hook)
	})
	if exitedDuringStartup {
		// onExit could not reconcile before the row existed. Now it does, after
		// every hook the provider emitted before dying has had its ordered chance
		// to update the ledger and prompt journal.
		u.reconcileRunnerExit(context.Background(), runnerID)
		return "", ErrProviderExitedDuringStartup
	}
	return runnerID, nil
}

type spawnPaths struct {
	crowbarHome string
	projectID   string
	repoID      string
	worktree    string
	tmpDir      string
}

func (u *runnerUsecase) spawnPaths(
	ctx context.Context,
	workspaceID, runnerID, providerID string,
) (spawnPaths, error) {
	crowbarHome, projectID, repoID, worktree, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: worktree dir: %w", err)
	}

	// The chats dir is resolved separately from the worktree/Cwd: for a home-kind
	// (adopted checkout) workspace the worktree is the user's REAL dir outside home,
	// so chat state (this tmp dir, the ledger) reroots under crowbar home while the
	// CLI still runs with Cwd = worktree.
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

	// Under the workspace's chats dir (always beneath crowbar home), keyed by the RUNNER —
	// id + provider — and NOT by the chat. The chat pointer is erasable (Displace clears it
	// while the process still runs), and a dir that can only be found through an erasable
	// pointer can never be reaped; see worktreepath.RunnerDir. This path is derivable from
	// a bare runner row forever, which is what lets BOTH removers find it: onExit below on a
	// clean death, and boot reconciliation when the daemon died before onExit could run.
	//
	// It holds the rendered hook config the CLI is pointed at, and must survive for the
	// whole life of that CLI — so it is removed on PTY death, never eagerly after spawn.
	//
	// It holds no secret: a provider owns its own credentials and Crowbar never copies
	// them anywhere (that rule is why CODEX_HOME is not ours, and why codex's sessions
	// survive a switch). Nothing in the descriptors puts a credential here, and nothing
	// may.
	tmpDir := worktreepath.RunnerDir(chatsDir, runnerID, providerID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: mkdir tmp: %w", err)
	}
	return spawnPaths{
		crowbarHome: crowbarHome,
		projectID:   projectID,
		repoID:      repoID,
		worktree:    worktree,
		tmpDir:      tmpDir,
	}, nil
}

func (u *runnerUsecase) renderSpawnContext(
	in spawnContext,
) (engineagents.TemplateCtx, bool) {
	tctx := engineagents.TemplateCtx{
		Tmp:         in.tmpDir,
		Cwd:         in.worktree,
		CrowbarHook: u.crowbarHookPath(in.crowbarHome),
		Segid:       in.runnerID,
		// The credential the descriptors hand `crowbar mcp` so this runner's tool
		// calls can be attributed to it. Minted here because this is where the
		// runner id is born, and by the SAME minter DispatchMCP verifies against —
		// a token minted anywhere else would authenticate nothing.
		RunnerToken: u.mintRunnerToken(in.runnerID),
		Provider:    in.providerID,
		ProjectID:   in.projectID,
		RepoID:      in.repoID,
		WorkspaceID: in.workspaceID,
		ChatID:      in.chatID,
		GapTurns:    strconv.Itoa(in.gapTurns),
		Message:     in.promptMessage,
		// Read by the descriptor's own model:/effort: apply steps and by nothing
		// else. They are empty on a chat that has chosen nothing, and the steps
		// that would read them are not rendered at all in that case.
		Model:  in.selection.Model,
		Effort: in.selection.Effort,
	}
	// The message for a provider that can ONLY be reached through a user message (a
	// resumed codex ignores every config channel). It POINTS at the ledger already on
	// disk and says where to start reading — it never carries the transcript, which
	// would dump the whole handed-off exchange into the chat for the user to scroll
	// past. An agent reads files.
	tctx.ContextPointer = engineagents.Expand(config.GetPrompts().HandoffPointer, tctx)
	// The preamble first (which tools exist), then the lineage (which OTHER chats
	// this one reads), then the handoff (what was said here already). Orientation
	// before history: a model should know it has get_chat_log and which ids to
	// point it at before it reads a conversation it is told not to act on.
	tctx.Context = composeContext(
		engineagents.Expand(config.GetPrompts().CapabilitiesInstruction, tctx),
		in.threads,
		in.conversation,
	)
	// WHETHER to deliver that document at all, which is a separate question from what
	// it says.
	//
	// A handoff is something that HAPPENED and is always worth delivering. The
	// capability preamble is standing orientation, so it may only drive delivery down a
	// SILENT channel: a fresh spawn's ContextInject is a config key or a flag, but a
	// resume's ResumeContextInject can be a USER MESSAGE — a resumed codex can be
	// reached no other way (see codex.yaml) — and reopening a closed tab resumes a
	// provider with nothing recorded in between. Letting the preamble deliver there
	// would open every revived codex chat with a "while you were away" pointer about
	// nothing, and codex answers its opening message on sight.
	//
	// The cost is that a provider whose resume channel IS silent (claude's is a flag)
	// also loses the preamble on a gapless revive. That is deliberate over the
	// alternative: whether a resume channel speaks out loud is the DESCRIPTOR's
	// knowledge, and inventing a "this channel is silent" field to recover one
	// directive would put a provider's manners in this package. Its tools are still
	// registered on that spawn either way — only the directive is missing.
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}

func (u *runnerUsecase) forkCLI(
	ctx context.Context,
	req forkRequest,
) (string, error) {
	if err := u.pendingHooks.register(req.runnerID); err != nil {
		u.agents.ForgetRunner(req.runnerID)
		RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)
		return "", fmt.Errorf("agent: spawn runner: install hook startup barrier: %w", err)
	}
	termSessID, err := u.term.CreateCommand(ctx, req.workspaceID, req.worktree, req.argv, req.env,
		u.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir))
	if err == nil {
		return termSessID, nil
	}
	u.pendingHooks.discard(req.runnerID)
	u.agents.ForgetRunner(req.runnerID)
	RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)

	// A CLI that is not installed is the ONE spawn failure the user can act on, so
	// it travels as its own sentinel (→ 424, a named message in the UI) rather than
	// being buried in a wrap chain that maps to an opaque 500. The provider id, not
	// the resolved argv[0], is what the UI can name.
	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return "", fmt.Errorf("%w: %s", engineterminal.ErrCommandNotFound, req.providerID)
	}
	return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
}

func (u *runnerUsecase) recordRunner(
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
		created, err := u.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: workspaceID,
			Now:         now,
		})
		if err != nil {
			return u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
		u.work.set(chatID, created.Working)
	}
	if _, err := u.runners.Start(ctx, agentrunner.StartInput{
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
		Now:          now,
	}); err != nil {
		return u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
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
	u.retireOthersOn(ctx, chatID, runnerID)
	return nil
}

func (u *runnerUsecase) mintRunnerToken(runnerID string) string {
	if u.minter == nil {
		return ""
	}
	return u.minter.Mint(runnerID)
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
}

type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
}

func (u *runnerUsecase) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	create bool,
) (spawnPreflight, error) {
	// This is the ONE seam every vendor CLI is launched through, which makes it the
	// only place a disabled provider can actually be stopped.
	if err := u.providers.requireProviderEnabled(ctx, providerID); err != nil {
		return spawnPreflight{}, err
	}
	// The tool switch is a SEPARATE axis from whether the provider is enabled: a CLI
	// spawned with its tools off still comes up, still fires its hooks and still
	// holds a normal chat.
	mcpOn, err := u.providers.providerMCPEnabled(ctx, providerID)
	if err != nil {
		return spawnPreflight{}, err
	}
	threads, err := u.chat.threadContext(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	// The selection is also what gets RECORDED on the runner, so the record and the
	// argv are rendered from ONE read: two reads could disagree across a concurrent
	// change and leave a process whose recorded selection is not the one it runs.
	sel, err := u.chat.chatSelection(ctx, chatID, create)
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

func (u *runnerUsecase) teardownAfterPersistFailure(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := u.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown after persist failure",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	return cause
}

func (u *runnerUsecase) onRunnerExit(home, runnerID, tmpDir string) func() {
	return func() {
		RemoveUnderHome(context.Background(), home, tmpDir)
		// CreateCommand can observe process exit before recordRunner has a
		// terminal-session id to persist. The startup barrier remembers that fact;
		// spawnRunner reconciles it immediately after persistence and ordered hook
		// replay, instead of this callback missing the not-yet-existing row forever.
		if u.pendingHooks.markExited(runnerID) {
			return
		}
		u.reconcileRunnerExit(context.Background(), runnerID)
	}
}

func (u *runnerUsecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}
