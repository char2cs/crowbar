// Package agent hosts the agentic-chat usecase. It owns two aggregates and the
// relationship between them: an AgentChat (a Crowbar thread, and the ledger on
// disk that is its only unique property) and an AgentRunner (one vendor CLI, in
// one PTY, that is currently pointed at one chat).
//
// The runner points at the chat. The chat never points back. That single arrow is
// what this package is built around: when a CLI moves between conversations — the
// user typing /clear or /resume INSIDE it — the move is ONE write to ONE aggregate
// (agentrunner.Move), and the chat being left is not written to at all. The
// previous model expressed the same move as EndSegment(A) + OpenSegment(B) across
// two aggregates with no transaction; it tore in half in production, committing the
// first and failing the second, and left chat A with no way back into it.
//
// Two rules govern everything here:
//
//   - PERSIST PLACEMENT, NEVER LIVENESS. Which chat and which conversation a runner
//     is on is Crowbar's own fact, so it is durable. Whether the CLI is ALIVE is the
//     PTY's fact and only the PTY's: "is this chat live?" is the query
//     LiveRunnerForChat (a row exists exactly while the process does), never a flag
//     we store and then have to keep true.
//
//   - RECONCILE, NEVER TRANSACT. By the time a hook reaches us the CLI has ALREADY
//     switched conversation. We cannot refuse it and cannot push it back, so the
//     hook path RECORDS what happened and must never fail on someone else's state.
//     Where two runners collide on one conversation, we act on the OTHER party
//     (evict it) rather than reject reality.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TerminalCommander is the terminal-engine seam the usecase spawns vendor CLIs
// through and tears them down with.
type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(),
	) (string, error)
	// TerminateGraceful quits a vendor CLI: a clean-exit SIGTERM, never SIGKILL.
	// It is the ONLY way this package ends a runner — we kill the process and let
	// its death carry the runner away (the engine's onExit → agentrunner.Exit →
	// the live row disappears), because the PTY is the sole authority on liveness
	// and asserting an Exit we have not observed would make us a second one.
	//
	// Graceful matters twice over: a well-behaved CLI flushes its native transcript
	// on SIGTERM, so neither the provider being switched out nor the runner being
	// EVICTED from a conversation loses its last turn — and an evicted runner's
	// conversation is about to be read by the runner taking it over. Applies
	// uniformly to every provider (Codex tolerates it too); no provider branching.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	// SessionLive reports whether a terminal session id is backed by a LIVE PTY right
	// now. It has NO caller in this package today, and it is kept deliberately: it is
	// the single authority BOOT RECONCILIATION asks, and boot reconciliation — which
	// Exits every runner whose PTY did not survive the restart — is the next task in
	// this series. A false here means the CLI is definitively gone, even though no
	// event ever recorded it.
	//
	// It is deliberately NOT the engine's SessionExists, which is also true for a
	// PTY-less suspended placeholder — a session whose process is already dead and
	// whose only remaining substance is scrollback on disk. Asking the registry "do
	// you know this id?" instead of "is this process alive?" is what let a
	// restart-orphaned chat keep advertising a live agent.
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool
}

// WorkspaceReader resolves a workspace's Crowbar-managed identity and git
// worktree directory, plus the directory that holds its agentic chat state.
type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	// AgentChatsDir returns the directory holding the workspace's agentic chat
	// state — the per-chat handoff ledger and the per-spawn tmp dirs (the rendered
	// hook config; nothing else — no descriptor copies any credential into them,
	// and none may). It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// so plaintext ledgers never land on the user's filesystem. The worktree/Cwd is
	// unaffected — WorktreeDir still returns it unchanged.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

// Usecase is the agentic-chat engine: spawning vendor CLIs, ingesting their hooks
// through the context-move reducer, moving the runner that results, and appending
// the ledger. It does NOT broadcast: every agentchat.* and agentrunner.* event is
// fanned out to the WS hub by the repository layer's hub projections, so the single
// source of lifecycle frames is the event stream. The usecase's job ends at issuing
// the command that emits the event.
type Usecase struct {
	chats    agentchat.EventStore
	runners  agentrunner.EventStore
	registry *engineagent.Registry
	term     TerminalCommander
	ws       WorkspaceReader
	// spawns serialises the USER-INITIATED spawn paths per chat (SpawnChat,
	// SwitchProvider, ResumeChat). See chatGate: it is the only thing that can stop two
	// concurrent switches putting two CLIs on one chat, and it is NEVER taken on the
	// hook path.
	spawns *chatGate
}

// New builds a Usecase over the two aggregates and the engine seams. registry is
// no longer a placement index — it holds only the per-spawn injected-context echo
// guard (see engineagent.Registry); every placement question is answered by the
// runner aggregate.
func New(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	registry *engineagent.Registry,
	term TerminalCommander,
	ws WorkspaceReader,
) *Usecase {
	return &Usecase{
		chats:    chats,
		runners:  runners,
		registry: registry,
		term:     term,
		ws:       ws,
		spawns:   newChatGate(),
	}
}

// SpawnChat creates a fresh AgentChat and starts a runner on it, launching the
// provider's vendor CLI in a PTY. The returned runnerID is the crowbarSegmentID
// every hook from that CLI carries, and it is stable for the life of the process —
// including across every conversation move it makes.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	defer u.spawns.lock(chatID)()

	runnerID, err = u.spawnRunner(ctx, chatID, workspaceID, providerID, nil, "", "", true, false, true)
	if err != nil {
		return "", "", err
	}
	return chatID, runnerID, nil
}

// RenameChat sets a chat's title under user>agent>derived precedence:
//
//	source "derived": set only if the title is currently empty (first-prompt fallback).
//	source "agent":   set unless the title is user-locked (agent may upgrade a derived title).
//	source "user"/"": set unconditionally AND lock (a manual rename wins and sticks).
//
// The empty-title-is-a-no-op and derived-only-if-empty gates live here (the
// SetTitle command only enforces the locked-vs-user rule). A successful change
// emits a title_set event, which the hub projection fans out as the lifecycle
// frame — the usecase no longer broadcasts.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: rename chat: get: %w", err)
	}
	switch source {
	case "derived":
		if chat.Title != "" {
			return nil
		}
	case "agent":
		if chat.TitleLocked {
			return nil
		}
	default: // "user" / "" — manual rename wins and locks
		source = "user"
	}
	if _, err := u.chats.SetTitle(ctx, chatID, title, source); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	return nil
}

// PurgeChat hard-deletes chatID via asynx Forget: the aggregate's event log AND
// its read-model row are erased, so a subsequent GetChat/ListChats genuinely
// reports not found. It then kills the CLI that was pointed at the chat, drops the
// chat's conversation history, and reaps its on-disk footprint.
//
// A runner may still be pointed at the chat, and a runner whose chat no longer exists is
// a runner with nowhere to write. It is DISPLACED (taken off the chat — a placement fact,
// which is ours) and then killed (a request, not an assertion). We do NOT delete its row
// from the read model: that would be Crowbar asserting a LIVENESS fact it does not own —
// the exact dual-authority mistake this refactor deletes. The row goes when the PTY does
// (TerminateGraceful → the engine's onExit → Exit → the projection drops it).
//
// ORDER: Forget the chat FIRST, kill the CLI SECOND. This is deliberate and it is
// the opposite of what the old code did.
//
//	The teardown fires reconcileRunnerExit asynchronously, off the terminal engine's
//	reap goroutine, and that path writes to the chat (it closes a turn the dead CLI
//	left open). asynx projections are async, so a chat command that COMMITS before the
//	Forget can have its read-model Save land AFTER Forget's row-delete — resurrecting
//	the deleted chat as a zombie row. That bug was found in live daemon testing, and
//	the old code fought it with an in-memory registry unbind (a synchronous guard the
//	teardown checked first). That registry is gone, so the guard has to be structural:
//	once the aggregate is Forgotten its event log is erased, so EVERY subsequent chat
//	command fails Validate (current == nil) and emits nothing at all. Forgetting first
//	makes the zombie unrepresentable rather than merely unlikely.
//
//	The window it opens instead is closed by IngestHook's chat-existence guards, which
//	cover BOTH hook kinds: a turn hook whose chat is gone is dropped, and so is a
//	conversation ANNOUNCEMENT (which would otherwise write a conversation-history row
//	against a chat that has just been deleted — a dangling /resume target). Either way
//	the CLI is killed again: it has nowhere left to write.
//
// Everything after the Forget is BEST-EFFORT: a terminate failure, a history-drop
// failure or a filesystem failure is logged and the purge still reports success.
// Wedging a delete the user asked for (leaving a chat they can never remove) is a
// worse outcome than an orphaned PTY or a leftover directory.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason the spawn paths take it: a delete racing a
	// switch would otherwise Start a CLI onto a chat that has just been Forgotten. That
	// self-heals (the runner's first hook finds no chat and retires it), but only after
	// spawning a real process and leaving its tmp dir behind. Serialising is a line.
	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	if err := u.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}
	u.retireChatRunner(ctx, chatID)

	// Drop the chat's conversation history. It is append-only and outlives the
	// process, so nothing else ever removes it — and a conversation still pointing
	// at a hard-deleted chat is a live trap: a later /resume of that session id would
	// resolve (ChatForSession) to a chat that no longer exists.
	if err := u.runners.ForgetChat(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation history (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}

	// Reap the chat's on-disk footprint (the handoff ledger + any residual per-spawn
	// tmp dir) now the aggregate is gone; a standalone hard delete would otherwise
	// leave this chat's PLAINTEXT ledger behind. The removal is routed through
	// RemoveUnderHome, which re-asserts the target is strictly under crowbar home, so
	// even a poisoned chats dir can never reach the user's real repository.
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve chats dir for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap guard (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	RemoveUnderHome(ctx, home, filepath.Join(chatsDir, chatID))
	return nil
}

// retireChatRunner takes the vendor CLI currently pointed at chatID off that chat and
// kills it. A dormant chat (no live runner) is a no-op — that absence is the answer, not
// an error. Best-effort by contract: every caller is a delete the user asked for, and
// none of them may be wedged by a failure here.
func (u *Usecase) retireChatRunner(
	ctx context.Context,
	chatID string,
) {
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		if !errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: look up live runner for chat (best-effort, continuing)",
				"chat_id", chatID, "err", err)
		}
		return
	}
	u.retire(ctx, live)
}

// retire takes a runner off the chat it is on and asks its process to quit — in that
// order, and the order is the point.
//
// DISPLACE FIRST. It cannot fail on anyone else's state, and it records the one thing we
// actually know: this CLI is no longer on that chat. It says NOTHING about whether the
// process is alive, so it is not a second opinion on liveness — it is a placement fact,
// and placement is ours alone. Recording it immediately is what makes "one runner per
// chat" true at every instant: a SIGTERM'd CLI does not die synchronously, and until it
// falls over it would otherwise still be pointed at a chat somebody else now owns —
// where it could write turns into that chat's ledger, and where a read could hand it out
// as the chat's live runner (attaching a pane to a corpse).
//
// KILL SECOND, best-effort. TerminateGraceful is a request; the PTY decides. If it fails
// the CLI stays alive — but it is already unplaced, so its hooks resolve to no chat and
// are dropped, and its death (whenever it comes) still carries its row away. A failure
// here therefore leaks a process, never a write.
func (u *Usecase) retire(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	if err := u.displace(ctx, runner); err != nil {
		// Best-effort: the runner is still on its chat, so its own exit will close any turn
		// it leaves open. We still kill it.
		slog.ErrorContext(ctx, "agent: retire runner: displace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
	}
	if err := u.term.TerminateGraceful(ctx, runner.TerminalSession); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: retire runner: terminate (best-effort, continuing)",
			"runner_id", runner.ID, "terminal_session_id", runner.TerminalSession, "err", err)
	}
}

// displace records that a runner is no longer placed on any chat, and then closes any turn
// it was leaving open on the chat it just left.
//
// Closing that turn HERE is not an opinion about liveness, and it has to be here. Turn
// state is normally repaired by the dying CLI's own exit (reconcileRunnerExit), which finds
// the chat through runner.CurrentChatID — the very field this command erases. Without this,
// a switch that aborts after the outgoing CLI was quit (an unknown target provider, a
// failed spawn) leaves the chat with no runner and Working=true FOREVER: the chat row
// spins, and the workspace's whole overlay spins with it, until the user resumes that chat
// and completes another turn. Once a runner is displaced, no hook of its can ever reach
// that chat again — so "nobody is working on this chat" is simply the last true thing we
// can say about it.
//
// An already-EXITED runner is not an error: a CLI quitting on its own moments before we
// went to take it off a chat is an ordinary, benign race, and the command tells us so with
// ErrValidation. Its exit has already cleared everything this would have.
func (u *Usecase) displace(
	ctx context.Context,
	runner domain.AgentRunner,
) error {
	vacated := runner.CurrentChatID

	if _, err := u.runners.Displace(ctx, runner.ID); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: displace runner: it had already exited (benign)",
				"runner_id", runner.ID, "chat_id", vacated)
			return nil
		}
		return fmt.Errorf("agent: displace runner: %w", err)
	}

	u.closeAbandonedTurn(ctx, vacated)
	return nil
}

// closeAbandonedTurn stops a turn on chatID when NOTHING is running on that chat any more —
// the turn's CLI is gone (dead, or displaced and dying) and no successor has taken the chat
// over, so the turn_stop hook that would have closed it is never coming.
//
// The guards are all "is there still someone whose turn this is?": a chat somebody else is
// now on (a provider switch spawns the incoming CLI before the outgoing one has fallen
// over) keeps ITS turn; a chat that no longer exists is not written to at all; and a chat
// that is not Working has nothing to close.
//
// An empty chatID means NOWHERE — a runner that is already displaced — and nowhere is never
// looked up.
func (u *Usecase) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
) {
	if chatID == "" {
		return
	}
	if _, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return // someone else is on this chat now: its turn is not ours to close
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		if !errors.Is(err, agentchat.ErrNotFound) {
			slog.WarnContext(ctx, "agent: close abandoned turn: get chat", "chat_id", chatID, "err", err)
		}
		return
	}
	if !chat.Working {
		return
	}
	if _, err := u.chats.StopTurn(ctx, chat.ID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: stop turn", "chat_id", chatID, "err", err)
	}
}

// deriveTitle turns a user prompt into a short chat title: the first non-empty
// line, trimmed, capped to 60 runes.
func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}

// spawnRunner is the single spawn seam: it launches providerID's vendor CLI in a
// PTY and records the runner that results, pointed at chatID. Every path that
// starts a CLI goes through it — SpawnChat (create=true, minting the chat too),
// SwitchProvider and ResumeChat (create=false, on a chat that already exists).
//
// It does ALL the IO first — resolve the descriptor, render the per-spawn tmp dir
// and hook config, build the spawn plan, launch the PTY — and only THEN issues the
// persistence commands. A pure command cannot spawn a process, so the CLI is
// necessarily live before the runner can be recorded; if either command fails, the
// just-spawned CLI is torn down so no orphan process leaks.
//
// conversation is the already-wrapped handoff document (the full ledger for a
// provider new to this chat, the gap only for one resumed into its own session, ""
// for a brand-new chat); injectTitle asks for the title instruction. Both are
// composed into the ONE {context} document the descriptor injects, because a
// provider may only have a single such channel — codex delivers title and handoff
// through the same developer_instructions key, so injecting them separately would
// have the second silently overwrite the first.
//
// resuming selects WHICH descriptor channel carries that document: ContextInject
// for a fresh session, ResumeContextInject for one resumed via session.resume. The
// distinction is real, not cosmetic — a resumed codex ignores every config channel
// and can only be reached through a user message (see codex.yaml) — and it is the
// descriptor, never this code, that knows what each provider needs.
func (u *Usecase) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	extraSteps []engineagent.InjectStep,
	conversation string,
	ledgerCut string,
	injectTitle bool,
	resuming bool,
	create bool,
) (string, error) {
	// The runner's id IS the crowbarSegmentID passed to every hook, minted here,
	// before the process exists — so a hook fired the instant the CLI comes up can
	// always name its runner. It is stable for the whole life of the process,
	// including across every conversation move: that stability is what makes a move
	// one write instead of a delete-here/insert-there.
	runnerID := uuid.NewString()

	crowbarHome, projectID, repoID, worktree, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: worktree dir: %w", err)
	}

	// The chats dir is resolved separately from the worktree/Cwd: for a home-kind
	// (adopted checkout) workspace the worktree is the user's REAL dir outside home,
	// so chat state (this tmp dir, the ledger) reroots under crowbar home while the
	// CLI still runs with Cwd = worktree.
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, providerID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}

	// Under the workspace's chats dir (always beneath crowbar home), keyed by
	// chatID+runnerID+providerID so it is deterministic per spawn. It holds the rendered
	// hook config the CLI is pointed at, and must survive for the whole life of that CLI
	// — so it is removed via onExit below (on PTY death), never eagerly after spawn.
	//
	// It holds no secret: a provider owns its own credentials and Crowbar never copies
	// them anywhere (that rule is why CODEX_HOME is not ours, and why codex's sessions
	// survive a switch). Nothing in the descriptors puts a credential here, and nothing
	// may.
	tmpDir := worktreepath.SegmentDir(chatsDir, chatID, runnerID, providerID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return "", fmt.Errorf("agent: spawn runner: mkdir tmp: %w", err)
	}

	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Segid:       runnerID,
		Provider:    providerID,
		Chatid:      chatID,
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: workspaceID,
		LedgerDir:   worktreepath.AgentLedgerDir(chatsDir, chatID),
		LedgerCut:   ledgerCut,
	}
	// The message for a provider that can ONLY be reached through a user message (a
	// resumed codex ignores every config channel). It POINTS at the ledger already on
	// disk and says where to start reading — it never carries the transcript, which
	// would dump the whole handed-off exchange into the chat for the user to scroll
	// past. An agent reads files.
	tctx.ContextPointer = engineagent.Expand(config.GetPrompts().HandoffPointer, tctx)
	// Compose the single {context} document: title instruction (only while the chat
	// has no title — a chat switched before it was ever titled would otherwise never
	// get one) followed by the handed-off conversation.
	var parts []string
	if injectTitle {
		parts = append(parts, engineagent.Expand(config.GetPrompts().TitleInstruction, tctx))
	}
	if conversation != "" {
		parts = append(parts, conversation)
	}
	tctx.Context = strings.Join(parts, "\n\n")

	steps := extraSteps
	if tctx.Context != "" {
		steps = append(steps, contextInject(descriptor, resuming)...)
	}

	// Register the injected document BEFORE the CLI can run: a provider whose only
	// resume channel is a user message (codex) fires its user-prompt hook with this
	// exact text the moment it starts, and that echo must never be recorded as a
	// ledger turn — that is what made handoffs nest inside themselves.
	u.registry.SetInjectedContext(runnerID, tctx.Context, tctx.ContextPointer)

	plan, err := engineagent.BuildSpawnPlan(descriptor, tctx, os.Environ(), steps)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}

	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)

	termSessID, err := u.term.CreateCommand(ctx, workspaceID, worktree, argv, plan.Env,
		u.onRunnerExit(crowbarHome, runnerID, tmpDir))
	if err != nil {
		// CreateCommand never got far enough to register onExit (which is what rm's
		// the tmp dir on a clean exit) — clean up here so a spawn failure doesn't leak
		// the runner's tmp dir. Guarded by crowbarHome so a poisoned chats dir can
		// never make this rm escape the user's real filesystem.
		RemoveUnderHome(ctx, crowbarHome, tmpDir)
		return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
	}

	now := time.Now()
	if create {
		if _, err := u.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: workspaceID,
			Now:         now,
		}); err != nil {
			return "", u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
	}
	if _, err := u.runners.Start(ctx, agentrunner.StartInput{
		RunnerID:        runnerID,
		WorkspaceID:     workspaceID,
		ProviderID:      providerID,
		TerminalSession: termSessID,
		ChatID:          chatID,
		Now:             now,
	}); err != nil {
		return "", u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
			fmt.Errorf("agent: spawn runner: start runner: %w", err))
	}
	return runnerID, nil
}

// teardownAfterPersistFailure kills a CLI that is already live but whose runner
// could not be recorded, so the failure leaves no orphan process behind — a CLI
// nothing in Crowbar points at is invisible, and an invisible agent is the worst
// state this system can be in. The original error is returned wrapped, so an
// ErrValidation still classifies as a conflict upstream. TerminateGraceful's own
// "session already gone" is harmless here and ignored.
func (u *Usecase) teardownAfterPersistFailure(
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

// RemoveUnderHome is the SINGLE guarded os.RemoveAll every agent-path filesystem
// removal routes through. It removes target ONLY when target is provably strictly
// under crowbar home — worktreepath.UnderHome, the exact strict-prefix check the
// worktree removers re-assert on their own root. AgentChatsDir already reroots chat
// state under home, but filepath.Join CLEANS "..", so a crafted repo remote slug
// (host/owner/../../..) could in principle collapse a derived path OUTSIDE home;
// this re-asserts the invariant AT the rm so no agent removal can EVER reach the
// user's real filesystem, even if a caller is handed a poisoned chats dir. A target
// not under home (including a blank/unresolvable home) is logged and skipped, never
// removed — fail-closed. Callers are all best-effort, so a plain rm error is logged,
// not returned.
//
// Exported so the workspace-delete cascade's on-disk reap seam (app.reapAgentChatFiles,
// wired into repositories.Container.ReapChatFiles) can route through the exact SAME
// guard PurgeChat uses, rather than a package outside agent reimplementing the check.
func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	if !worktreepath.UnderHome(target, home) {
		slog.WarnContext(ctx, "agent: refusing to rm agent path outside crowbar home (skipping)",
			"target", target, "home", home)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		slog.WarnContext(ctx, "agent: reap agent path", "target", target, "err", err)
	}
}

// onRunnerExit builds the CreateCommand onExit callback for runnerID: the PTY has
// died, which is the ONE fact that makes a runner dead. home is captured at spawn
// time (the same home tmpDir was created under) so the guard needs no post-hoc
// lookup that a since-deleted workspace could fail. It runs on a background context:
// the terminal engine invokes onExit from its own reap goroutine, well after any
// request context that spawned this runner could have been cancelled.
func (u *Usecase) onRunnerExit(home, runnerID, tmpDir string) func() {
	return func() {
		RemoveUnderHome(context.Background(), home, tmpDir)
		u.reconcileRunnerExit(context.Background(), runnerID)
	}
}

// reconcileRunnerExit records the death of a CLI. It is the ONLY place a runner is
// Exited at runtime, and it runs because the PROCESS died — never because Crowbar
// formed an opinion that it should be dead. Every teardown path in this package
// (provider switch, eviction, chat delete, workspace delete) converges here through
// the same door: they all just SIGTERM the PTY.
//
// It also closes a turn the dead CLI left open. A CLI that dies mid-turn will never
// send its turn_stop hook, so without this the chat stays Working forever — a
// spinner that never stops, on a chat with nothing running.
//
// That turn is closed ONLY if the chat has no OTHER runner on it now. A provider
// switch starts the incoming CLI while the outgoing one is still dying, so a belated
// exit must never close the INCOMING runner's turn — and once the runner's own row is
// gone (Exit, above), "is anyone still on this chat" is exactly the query
// LiveRunnerForChat. The old code answered the same question with "am I still the
// chat's ACTIVE SEGMENT", which is the stored-liveness flag this refactor deletes.
//
// Errors are logged, not returned: onExit runs off the terminal engine's reap
// goroutine and there is no caller to hand an error to.
func (u *Usecase) reconcileRunnerExit(ctx context.Context, runnerID string) {
	// The echo guard is per-spawn and means nothing once the process is gone.
	u.registry.ForgetRunner(runnerID)

	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if !errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: reconcile runner exit: get runner", "runner_id", runnerID, "err", err)
		}
		// Already exited (a double exit is not an error — the row is simply gone).
		return
	}
	if _, err := u.runners.Exit(ctx, runnerID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: reconcile runner exit: exit runner", "runner_id", runnerID, "err", err)
		return
	}

	// Close a turn it left open — unless it had already been DISPLACED, in which case its
	// chat (if it still had a turn to close) was dealt with at displacement time and
	// CurrentChatID is now empty, meaning nowhere.
	u.closeAbandonedTurn(ctx, runner.CurrentChatID)
}

func (u *Usecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}

// IngestHook maps an incoming vendor hook to a canonical event and applies it: a
// session_start goes to the context-move reducer, a user_prompt opens the turn and
// appends it to the ledger, a turn_stop closes the turn and appends the reply.
//
// EVERY hook is routed by resolving runnerID → runner → CurrentChatID from DURABLE
// state. Nothing in memory can disagree with it, because there is no longer anything
// in memory to disagree: this is what stops an orphaned CLI's turns landing in a chat
// it has left (spec §4.7). A hook from a runner we do not know, or for a chat that no
// longer exists, or with a malformed payload, is ignored — never an error. A hook must
// never break the vendor CLI's turn.
func (u *Usecase) IngestHook(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if errors.Is(err, agentrunner.ErrNotFound) {
			// A hook from a runner we do not know (or one whose PTY has already died):
			// ignore it. Never resurrect a runner from a hook.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: runner: %w", err)
	}

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The runner is the source of truth for which provider spawned this CLI. The
	// hook's self-reported provider is only a guard against a mis-authored descriptor.
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, runner.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	payload, err := descriptor.ParsePayload(rawPayload)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: parse payload", "err", err, "runner_id", runnerID)
		return nil
	}

	ev, _ := descriptor.MapHook(canonicalEvent, payload)

	switch ev.Kind {
	case "session_start":
		return u.handleSessionStart(ctx, runner, ev)
	case "user_prompt", "turn_stop":
		return u.handleTurn(ctx, runner, ev)
	}
	return nil
}

// handleTurn applies a turn hook to the chat the runner is on RIGHT NOW — read from
// the runner, which the preceding session_start has already moved if the CLI changed
// conversation (a provider announces the switch BEFORE the turn that follows it, so
// no turn is ever misfiled).
func (u *Usecase) handleTurn(
	ctx context.Context,
	runner domain.AgentRunner,
	ev engineagent.CanonicalEvent,
) error {
	if runner.CurrentChatID == "" {
		// The runner is placed NOWHERE: Crowbar has taken it off its chat and is killing
		// it (a switch, an eviction, a chat deleted under it), and a SIGTERM'd CLI keeps
		// talking for a moment. Its turns belong to nobody, and nowhere is never looked up
		// — GetChat("") would miss and trigger agentchat's lazy self-heal, replaying the
		// ENTIRE event log, on every hook of a dying CLI.
		return nil
	}

	chat, err := u.chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			// The chat was deleted out from under the CLI (which is still dying). A turn
			// typed into a chat the user has just removed goes nowhere, by design.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: chat: %w", err)
	}

	switch ev.Kind {
	case "user_prompt":
		// Crowbar's own context document coming back at us: a provider whose only
		// resume channel is a user message (codex) fires user_prompt with the very
		// handoff we injected. That is not something the user said — recording it would
		// put the handoff in the ledger as a "user" turn, and the NEXT handoff would
		// then quote it inside itself (the nesting seen live). Drop it from the ledger
		// and from title derivation, but still open the turn: the CLI really is working
		// on it, and the workspace's working overlay must say so.
		if u.registry.ConsumeInjectedContext(runner.ID, ev.Message) {
			if _, err := u.chats.StartTurn(ctx, chat.ID, time.Now()); err != nil {
				return fmt.Errorf("agent: ingest hook: start turn: %w", err)
			}
			return nil
		}
		if err := u.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
		}
		// A user prompt opens the turn: mark the chat Working so the read model (and
		// the workspace spinner) see a live turn.
		if _, err := u.chats.StartTurn(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		return u.appendTurn(ctx, chat, runner.ProviderID, "user", ev.Message)

	case "turn_stop":
		// The turn ended: clear Working. Issued before the ledger append so the
		// live-state event lands even when the assistant message is empty (an empty
		// message is a ledger no-op, not a turn-state no-op).
		if _, err := u.chats.StopTurn(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: stop turn: %w", err)
		}
		return u.appendTurn(ctx, chat, runner.ProviderID, "assistant", ev.Message)
	}
	return nil
}

// handleSessionStart reconciles a conversation announcement. By the time we are
// called the CLI has ALREADY switched — we RECORD what happened, and we never fail
// on anybody else's state (spec §3).
func (u *Usecase) handleSessionStart(
	ctx context.Context,
	runner domain.AgentRunner,
	ev engineagent.CanonicalEvent,
) error {
	if ev.SessionID == "" {
		return nil
	}

	// "Is this conversation one we know?" is answered from APPEND-ONLY history, so it
	// keeps answering long after the runner that opened the conversation has died —
	// which is what makes a /resume into a dormant chat recognisable instead of
	// looking brand new. It replaces the in-memory sessionToChat map AND the boot
	// reseed that used to repopulate it.
	knownChatID, err := u.runners.ChatForSession(ctx, runner.WorkspaceID, ev.SessionID)
	known := err == nil
	if err != nil && !errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("agent: ingest hook: lookup session: %w", err)
	}

	switch d := engineagent.Decide(runner.CurrentSession, ev.SessionID, knownChatID, known); d.Kind {
	case engineagent.MoveNoop:
		return nil
	case engineagent.MoveBind:
		// The conversation is recorded AGAINST THE CHAT the runner is on, so that chat
		// had better still be there. It might not be: PurgeChat kills the CLI but a
		// SIGTERM is not synchronous, so a chat deleted seconds after it was created
		// ("wrong provider, undo") can be gone before its CLI has even announced. Binding
		// anyway would write a conversation-history row against a hard-deleted chat — a
		// dangling /resume target, the exact live trap PurgeChat drops that history to
		// prevent.
		ok, err := u.requirePlacement(ctx, runner, runner.CurrentChatID)
		if err != nil || !ok {
			return err
		}
		if _, err := u.runners.BindSession(ctx, runner.ID, ev.SessionID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: bind session: %w", err)
		}
		return nil
	case engineagent.MoveToNew:
		return u.moveToNewChat(ctx, runner, ev.SessionID)
	case engineagent.MoveToKnown:
		// The destination is resolved from append-only history, which can outlive the chat
		// it names: PurgeChat's history drop is best-effort, and deleting a Crowbar chat
		// deliberately does NOT delete the vendor's session file — so a purged chat's
		// conversation can still be sitting in the CLI's own /resume picker. Pick it, and
		// an unguarded move would repoint the runner at a chat that does not exist: an
		// invisible CLI, writing nowhere, forever.
		ok, err := u.requirePlacement(ctx, runner, d.ChatID)
		if err != nil || !ok {
			return err
		}
		return u.moveToKnownChat(ctx, runner, d.ChatID, ev.SessionID)
	}
	return nil
}

// requirePlacement reports whether chatID still exists — i.e. whether it is somewhere a
// runner can legitimately be placed and written to.
//
// When it does not, the runner has nowhere to write and must not be left running: it is
// retired (taken off whatever it was pointed at, and asked to quit). The hook itself is
// DROPPED, never failed — a hook must never break the vendor CLI's turn, and there is
// nothing the CLI could do with an error about a chat the user deleted anyway.
//
// This is not the reducer refusing reality (spec §3): the CLI's conversation switch is
// still a fait accompli. It is Crowbar declining to invent a placement — recording a
// runner onto a chat that does not exist is not a truth about the world, it is a lie the
// read model would then have to live with.
func (u *Usecase) requirePlacement(
	ctx context.Context,
	runner domain.AgentRunner,
	chatID string,
) (bool, error) {
	if chatID == "" {
		// Already displaced (we are killing it, and it has not fallen over yet).
		u.retire(ctx, runner)
		return false, nil
	}
	_, err := u.chats.GetChat(ctx, chatID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, agentchat.ErrNotFound) {
		return false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	slog.WarnContext(ctx, "agent: ingest hook: the runner's chat no longer exists; dropping the announcement and retiring the CLI",
		"runner_id", runner.ID, "chat_id", chatID)
	u.retire(ctx, runner)
	return false, nil
}

// moveToNewChat handles /clear and /new: a conversation nobody has ever seen appeared
// under a live runner, so a chat is minted for it and the runner moves in.
//
// The create runs FIRST because a create cannot destroy anything: if the Move then
// fails, the worst outcome is a stray empty chat in the sidebar — annoying,
// self-evident, deletable. The chat the runner is LEAVING is not written to at all, so
// no failure here can damage it (spec §4.2). This is the one flow that is still two
// writes, and the ordering is what bounds its failure.
func (u *Usecase) moveToNewChat(
	ctx context.Context,
	runner domain.AgentRunner,
	sessionID string,
) error {
	newChatID := uuid.NewString()
	if _, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          newChatID,
		WorkspaceID: runner.WorkspaceID,
		Now:         time.Now(),
	}); err != nil {
		return fmt.Errorf("agent: ingest hook: mint chat: %w", err)
	}
	if _, err := u.runners.Move(ctx, runner.ID, newChatID, sessionID, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to new chat: %w", err)
	}
	return nil
}

// moveToKnownChat handles /resume into a conversation we know: the runner is repointed
// at the chat that owns it, and any runner already HOLDING that conversation is evicted.
//
// The eviction is forced by invariant I3 — at most one live runner per conversation,
// because two CLIs on one provider session id both write the same session file and
// corrupt it. That constraint is the PROVIDER's, not ours, and the CLI has already
// joined the conversation, so evicting the incumbent is the only move available.
//
// ORDER: Move FIRST (record reality — one write, one aggregate, and it cannot fail on
// anyone else's state), evict SECOND. If the kill then fails, our record is still
// ACCURATE — two runners really do hold the conversation — and only the cleanup needs a
// retry. The reverse order, ending something before recording reality, is literally the
// bug that bricked a chat.
func (u *Usecase) moveToKnownChat(
	ctx context.Context,
	runner domain.AgentRunner,
	toChatID string,
	sessionID string,
) error {
	incumbents := u.incumbentsOf(ctx, runner, toChatID, sessionID)

	if _, err := u.runners.Move(ctx, runner.ID, toChatID, sessionID, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to known chat: %w", err)
	}

	for _, inc := range incumbents {
		u.evict(ctx, inc)
	}
	return nil
}

// incumbentsOf collects every runner the move is about to displace: the one HOLDING THE
// CONVERSATION (invariant I3) and the one PLACED ON THE CHAT (invariant I2). They are
// usually the same runner, and often there is none — but they are two different questions,
// and answering only the first left the reported bug alive with one variable changed:
//
//	Chat B is being worked by codex, which has not announced its conversation yet. B's
//	older claude conversation is still in Crowbar's history AND still in claude's own
//	/resume picker (Crowbar never deletes a vendor's session file). Resume it from another
//	chat's CLI: nobody HOLDS that conversation, so nothing was evicted — and chat B ended
//	up with two live CLIs on it, indefinitely, both writing its ledger, one invisible.
//
// Placement onto a chat must evict whoever else is placed there. That is what makes I2 an
// invariant rather than a coincidence, and it is also the only thing that covers a
// concurrent move the spawn gate cannot reach (a hook-path move onto B landing while a
// gated SwitchProvider(B) is spawning — a hook is never gated, and never may be).
//
// A lookup failure is logged, never fatal: we still record what the CLI has already done.
// At worst an incumbent survives and the (already logged) cleanup is owed.
func (u *Usecase) incumbentsOf(
	ctx context.Context,
	runner domain.AgentRunner,
	toChatID string,
	sessionID string,
) []domain.AgentRunner {
	var out []domain.AgentRunner
	seen := map[string]bool{runner.ID: true} // never evict yourself

	add := func(inc domain.AgentRunner, err error, what string) {
		if err != nil {
			if !errors.Is(err, agentrunner.ErrNotFound) {
				slog.ErrorContext(ctx, "agent: ingest hook: look up incumbent runner",
					"by", what, "chat_id", toChatID, "session_id", sessionID, "err", err)
			}
			return
		}
		if seen[inc.ID] {
			return
		}
		seen[inc.ID] = true
		out = append(out, inc)
	}

	bySession, err := u.runners.LiveRunnerForSession(ctx, runner.WorkspaceID, sessionID)
	add(bySession, err, "conversation")

	byChat, err := u.runners.LiveRunnerForChat(ctx, toChatID)
	add(byChat, err, "chat")

	return out
}

// evict retires a runner whose conversation another runner has just taken over
// (invariant I3): it is taken off that chat and asked to quit. Its PTY's death is still
// what makes it DEAD — the Exit follows from the engine's onExit callback, never from an
// assertion here.
//
// The displacement is the part that must not be skipped, and it is why this is `retire`
// and not a bare kill. If the kill fails the evictee is still alive, and an evictee that
// is still POINTED at the conversation it lost would go on appending its turns to the
// ledger of the chat the mover now owns — two CLIs writing one conversation, which is the
// corruption I3 exists to prevent. Unplaced, its hooks resolve to no chat and are dropped.
//
// Nothing here is returned: by the time we get here the move is already recorded, and the
// record is truthful either way. An eviction failure must never propagate back as a failed
// hook — the CLI has already switched, and there is nothing for it to do with an error.
func (u *Usecase) evict(ctx context.Context, incumbent domain.AgentRunner) {
	u.retire(ctx, incumbent)
}

// appendTurn records one conversation turn (user or assistant) into the chat's
// ledger. Empty text is a no-op. The turn's lifecycle frame is emitted by the
// StartTurn/StopTurn events the caller issues (fanned out by the hub projection),
// not from here — the ledger is a content log, not an aggregate.
//
// The turn is tagged with the provider of the RUNNER that produced it, which is what
// lets a later handoff say "assistant (codex): …" and lets the resume path ask when a
// given provider last spoke.
func (u *Usecase) appendTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	role, text string,
) error {
	if text == "" {
		return nil
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: chats dir: %w", err)
	}
	dir := worktreepath.AgentLedgerDir(chatsDir, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: ledger open: %w", err)
	}
	if _, err := led.AppendTurn(role, providerID, time.Now(), text); err != nil {
		return fmt.Errorf("agent: ingest hook: ledger append: %w", err)
	}
	return nil
}

// AssembleHandoff resolves chatID's ledger directory, reads every entry, and wraps
// them in a legible preamble/footer so a freshly spawned provider CLI can be handed
// the prior context. Returns "" (not an error) when the ledger has no entries yet.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.assembleConversation(ctx, chatID, false, time.Time{})
}

// SwitchProvider is the headline provider handoff: it quits the chat's current
// vendor CLI, assembles a handoff from the ledger, and starts targetProviderID as a
// NEW runner on the SAME chat with that handoff injected. The chat does not move and
// is not written to; only the runner pointed at it changes.
//
// If targetProviderID left a conversation behind in this chat AND actually spoke in
// it, the new CLI is resumed into that very conversation (session.resume) and handed
// only the GAP — what happened under other providers while it was away. Otherwise it
// is spawned fresh with the whole ledger.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	defer u.spawns.lock(chatID)()
	return u.switchProviderLocked(ctx, chatID, targetProviderID)
}

// switchProviderLocked is SwitchProvider's body, with the chat's spawn gate ALREADY
// held. ResumeChat holds the same gate and calls straight in here — the gate is not
// re-entrant, and a revive that took it twice would deadlock on itself.
func (u *Usecase) switchProviderLocked(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: chat: %w", err)
	}

	priorSessionID, leftAt, err := u.resumableConversation(ctx, chat, targetProviderID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: resumable conversation: %w", err)
	}
	resuming := priorSessionID != ""

	// Read-BEFORE-terminate: the ledger is built from hooks and is already on disk, so
	// assembling the handoff never depends on the outgoing CLI still being alive — and
	// doing it FIRST means a failure here aborts the switch with nothing destroyed,
	// rather than leaving the chat with its old CLI killed and the new one spawned with
	// an EMPTY handoff.
	//
	// A provider resumed into its OWN conversation already holds every turn up to the
	// moment it was switched out, so it is handed only the gap. Replaying the whole
	// ledger to it would duplicate its own history back at it — noise that dilutes the
	// very turns it is meant to notice. A provider new to this chat has no history at
	// all, so it gets the whole conversation.
	conversation, err := u.assembleConversation(ctx, chatID, resuming, leftAt)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
	}

	// Where a resumed provider should START reading the ledger: the last turn it
	// already saw. It is POINTED at the conversation on disk rather than handed a copy
	// of it (see TemplateCtx.ContextPointer).
	var ledgerCut string
	if resuming {
		ledgerCut, err = u.ledgerCut(ctx, chat, leftAt)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: ledger cut: %w", err)
		}
	}

	if err := u.quitOutgoingCLI(ctx, chatID); err != nil {
		return "", err
	}

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: worktree dir: %w", err)
	}
	d, err := engineagent.ResolveDescriptor(crowbarHome, targetProviderID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
	}

	// Resume arg must be split into separate argv tokens: exec.Command does NOT split
	// a string on whitespace, so a whole "--resume {id}" template handed to a single
	// pass_arg would become one literal argument.
	var resumeSteps []engineagent.InjectStep
	if resuming && d.Session.Resume != nil && d.Session.Resume.Arg != "" {
		resumeCtx := engineagent.TemplateCtx{ID: priorSessionID}
		for _, tok := range strings.Fields(engineagent.Expand(d.Session.Resume.Arg, resumeCtx)) {
			resumeSteps = append(resumeSteps, engineagent.InjectStep{
				Verb: "pass_arg",
				Args: map[string]any{"positional": tok},
			})
		}
	}

	// Resume args go first so codex's `resume <id>` subcommand precedes any positional
	// context; order is irrelevant for claude's flag pair.
	return u.spawnRunner(
		ctx, chatID, chat.WorkspaceID, targetProviderID,
		resumeSteps, conversation, ledgerCut, chat.Title == "", resuming, false,
	)
}

// quitOutgoingCLI gracefully quits the CLI currently pointed at chatID, so a
// well-behaved one — Claude Code in particular — gets the chance to flush its native
// transcript before it dies; a hard kill can lose its last pre-switch turn.
//
// A DORMANT chat is not an error: there is simply no outgoing CLI to quit. Such a chat
// used to be a dead end (the pane told the user to "switch provider to start a new one"
// while the switch returned ErrNotFound), so reviving a dead chat runs this exact path —
// that is all ResumeChat is.
//
// A failed terminate is SURFACED, not swallowed: if the outgoing CLI still exists but
// could not be killed, proceeding would leave TWO live CLIs in the same worktree, both
// pointed at this chat. The one error the terminal engine returns today
// (ErrSessionNotFound) means the session is already gone — safe, even correct, to
// continue: the alternative would trap a chat unable to ever switch again once its CLI
// exits on its own.
//
// ORDER — and it is the reverse of every other teardown in this file, deliberately.
// Elsewhere we DISPLACE first (record the placement fact, which cannot fail) and kill
// second. Here the kill is allowed to ABORT the whole switch, so displacing first would
// take the CLI off its chat and then leave it there, alive, holding a chat that now
// believes it has nobody — Crowbar lying about its own placement, which is the one thing
// placement is not allowed to do. So: kill first, and displace only once the CLI is
// definitely going away.
//
// Displacing at all is what closes the window an ordering heuristic cannot: the outgoing
// CLI may not have announced its conversation yet (claude takes about a second, and the
// user can click Switch well inside that). If it announces AFTER the incoming CLI has
// started, it would stamp a fresher timestamp than the incoming runner's spawn — and any
// "whoever arrived last holds the chat" rule would hand the chat back to the corpse.
// Unplaced, it announces into nothing and is dropped.
//
// The runner is NOT Exited here. Its PTY's death does that (onExit → reconcileRunnerExit),
// because the PTY is the only thing that knows.
func (u *Usecase) quitOutgoingCLI(
	ctx context.Context,
	chatID string,
) error {
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // dormant: nothing to quit
	}
	if err != nil {
		return fmt.Errorf("agent: switch provider: live runner: %w", err)
	}
	if err := u.term.TerminateGraceful(ctx, live.TerminalSession); err != nil {
		if !errors.Is(err, engineterminal.ErrSessionNotFound) {
			// The CLI is still on its chat, and it stays there: the switch is aborted with
			// nothing changed rather than half-done.
			return fmt.Errorf("agent: switch provider: terminate outgoing terminal: %w", err)
		}
		slog.WarnContext(ctx, "agent: switch provider: outgoing terminal session already gone before terminate; continuing switch",
			"chat_id", chatID, "runner_id", live.ID, "terminal_session_id", live.TerminalSession, "err", err)
	}
	// A failed displace ABORTS the switch, and this is the one teardown where it must: the
	// caller's very next act is to spawn the incoming CLI, so continuing would place a
	// second runner on a chat the first one is still recorded on — the two-live-CLIs state
	// this whole model exists to make unrepresentable. Aborting is cheap here and costs the
	// user nothing they cannot get back: the outgoing CLI is already dead or dying, so the
	// chat simply drops to dormant when its PTY goes, and Resume revives it.
	if err := u.displace(ctx, live); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	return nil
}

// ResumeChat brings a dormant chat back: its CLI exited, or it died with the daemon
// (agent PTYs are command sessions and never survive a restart). "Dormant" is a
// QUERY — no runner points at the chat — not a flag, so there is no state that could
// say "dormant" while a CLI is demonstrably still running.
//
// Everything needed to revive it is in the chat's conversation history: the provider
// that was last here, and the conversation it was in. So a revive IS a switch to that
// provider — the same path, which finds the conversation and resumes into it, leaving
// the CLI exactly where the user left it.
//
// A chat that still has a live runner is returned as-is (no-op): reviving it would
// tear down a perfectly good CLI and spawn a second one on the same conversation. That
// check and the spawn that may follow it are under the chat's spawn gate, so two clicks
// on Resume cannot both conclude "dormant" and both spawn.
func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: resume chat: live runner: %w", err)
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: no conversation to resume: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	return u.switchProviderLocked(ctx, chatID, last.ProviderID)
}

// resumableConversation picks the conversation targetProviderID should be resumed
// into, and the moment it left (the cut for the "while you were away" gap). Returns
// "" when there is nothing resumable — the provider is new to this chat, or it never
// actually said anything here — and the caller then spawns it fresh with the whole
// conversation instead.
//
// The second case is the subtle one, and it shipped as a bug: A SESSION ID IS NOT A
// CONVERSATION. A vendor CLI reports its session id the instant it starts (our
// session_start hook records it), but only WRITES that conversation once there is at
// least one message. Resuming such an id fails outright — claude dies on startup with
// "No conversation found with session ID: <id>", which is exactly what a user saw
// after opening a chat they had never sent a message in. So the conversation history
// alone is not enough: the LEDGER must show the provider actually said something.
//
// That same ledger read gives us the cut. The moment a provider "left" is the moment
// it last spoke — which is more honest than any timestamp on the conversation row,
// and it degrades gracefully: whatever happened in this chat after that turn is, by
// definition, what it missed.
func (u *Usecase) resumableConversation(
	ctx context.Context,
	chat domain.AgentChat,
	targetProviderID string,
) (sessionID string, leftAt time.Time, err error) {
	convs, err := u.runners.ConversationsForChat(ctx, chat.ID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("conversations: %w", err)
	}
	// Oldest first, so the LAST match is the most recent conversation this provider
	// held in this chat.
	for _, c := range convs {
		if c.ProviderID == targetProviderID {
			sessionID = c.SessionID
		}
	}
	if sessionID == "" {
		return "", time.Time{}, nil
	}

	led, err := u.openLedger(ctx, chat)
	if err != nil {
		return "", time.Time{}, err
	}
	leftAt, err = led.LastTurnAt(targetProviderID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("ledger last turn: %w", err)
	}
	if leftAt.IsZero() {
		// The CLI reported this conversation id but never recorded a turn under it, so
		// there is no conversation on disk to resume. Spawn fresh.
		slog.InfoContext(ctx, "agent: prior conversation has no recorded turns; spawning fresh instead of resuming",
			"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID)
		return "", time.Time{}, nil
	}
	return sessionID, leftAt, nil
}

// ledgerCut names the last ledger turn recorded before the provider left — the point
// a resumed provider should start reading from. Empty when it saw nothing.
func (u *Usecase) ledgerCut(
	ctx context.Context,
	chat domain.AgentChat,
	leftAt time.Time,
) (string, error) {
	led, err := u.openLedger(ctx, chat)
	if err != nil {
		return "", err
	}
	return led.LastEntryAt(leftAt)
}

// openLedger opens the chat's handoff ledger. It resolves the chats dir itself
// rather than taking a worktree path: the ledger always lives under the workspace's
// chats dir, which for a home-kind / adopted-checkout workspace is rerooted under
// crowbar home, NOT beside the user's real worktree.
func (u *Usecase) openLedger(
	ctx context.Context,
	chat domain.AgentChat,
) (*ledger.Ledger, error) {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("chats dir: %w", err)
	}
	led, err := ledger.Open(worktreepath.AgentLedgerDir(chatsDir, chat.ID))
	if err != nil {
		return nil, fmt.Errorf("ledger open: %w", err)
	}
	return led, nil
}

// assembleConversation renders the handoff document for a spawning provider:
// gap-only (turns recorded after leftAt) wrapped in HandoffResumeWrapper when it is
// being resumed into its own conversation, the whole ledger wrapped in HandoffWrapper
// when it is new to the chat. Empty (not an error) when there is nothing to hand over
// — a brand-new chat, or a revive where nothing happened while the CLI was gone; the
// caller then injects no context document at all.
func (u *Usecase) assembleConversation(
	ctx context.Context,
	chatID string,
	resuming bool,
	leftAt time.Time,
) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: assemble conversation: chat: %w", err)
	}
	led, err := u.openLedger(ctx, chat)
	if err != nil {
		return "", fmt.Errorf("agent: assemble conversation: %w", err)
	}

	wrapper := config.GetPrompts().HandoffWrapper
	render := led.RenderConversation
	if resuming {
		wrapper = config.GetPrompts().HandoffResumeWrapper
		render = func() ([]byte, error) { return led.RenderConversationAfter(leftAt) }
	}

	blob, err := render()
	if err != nil {
		return "", fmt.Errorf("agent: assemble conversation: ledger read: %w", err)
	}
	if len(blob) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(wrapper, "{conversation}", string(blob)), nil
}

// contextInject picks the descriptor channel that carries the {context} document:
// the resume channel when the CLI is being resumed into its own native session, the
// fresh-spawn channel otherwise. Which argv/config/file mechanism each of those
// actually is — and why they differ — is the descriptor's knowledge, never this
// package's.
func contextInject(d *engineagent.Descriptor, resuming bool) []engineagent.InjectStep {
	if resuming {
		return d.ResumeContextInject
	}
	return d.ContextInject
}

// ListProviders enumerates the registered agent providers for the workspace's
// crowbar home (embedded defaults + on-disk overrides), backing GET
// .../agent/providers. workspaceID is only used to resolve crowbar home — the
// descriptor set is global — so any workspace in the same home yields the same list.
func (u *Usecase) ListProviders(
	ctx context.Context,
	workspaceID string,
) ([]engineagent.Descriptor, error) {
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: worktree dir: %w", err)
	}
	descs, err := engineagent.AllDescriptors(crowbarHome)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: %w", err)
	}
	out := make([]engineagent.Descriptor, 0, len(descs))
	for _, d := range descs {
		out = append(out, *d)
	}
	return out, nil
}

// ListChats returns every AgentChat.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.chats.ListChats(ctx)
}

// ListChatsByWorkspace returns every AgentChat anchored to workspaceID.
func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.AgentChat, error) {
	return u.chats.ListByWorkspace(ctx, workspaceID)
}

// GetChat returns a single AgentChat by id.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	return u.chats.GetChat(ctx, id)
}

// LiveRunnerForChat returns the runner currently pointed at chatID. ErrNotFound
// means the chat is DORMANT — a real answer, not a failure. Row-existence is the
// liveness answer, because the row exists exactly while the PTY does.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

// ConversationsForChat returns every conversation the chat has hosted, oldest
// first — the append-only history that replaced the chat's embedded segments.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}
