// Package agent hosts the agentic-chat usecase: it owns AgentChat lifecycle
// (segments embedded in the aggregate), spawns vendor CLIs in a PTY, runs the
// context-move reducer against incoming hooks, and appends the ledger. Every
// persistence mutation is a command against the asynx-backed agentchat
// EventStore — the usecase reads current state, lets the reducer/engine decide
// the outcome, performs any IO (spawn/terminate PTY) OUTSIDE the command, then
// issues the command that emits the event.
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

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
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
	// TerminateGraceful gracefully quits the outgoing CLI on a provider switch
	// (spec §8): a clean-exit SIGTERM, not SIGKILL — Claude flushes its native
	// transcript on a clean exit, and a hard kill can lose the outgoing CLI's
	// last pre-switch turn. Applies uniformly to every provider (Codex
	// tolerates it too); no provider branching here.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	// SessionExists reports whether a terminal session id is still live in the
	// terminal engine's registry (true for both an attached PTY and a suspended
	// placeholder). ReconcileOnBoot uses it as the "is this CLI process still
	// around" liveness check for a chat's active segment: a false here after a
	// daemon crash means the segment's process is definitely gone even though no
	// event ever recorded that.
	SessionExists(
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
	// state — the per-chat handoff ledger and per-segment tmp dirs (rendered hook
	// config + any codex auth.json copy). It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// (spec §3.5, Task 7) so plaintext ledgers never land on the user's
	// filesystem. The worktree/Cwd is unaffected — WorktreeDir still returns it
	// unchanged.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

// Usecase is the agentic-chat engine: spawning vendor CLI segments, ingesting
// their hooks through the context-move reducer, and persisting the result via
// the asynx-backed agentchat EventStore. It does NOT broadcast lifecycle events
// itself: every agentchat.* event is fanned out to the WS hub by the repository
// layer's hub projection (repositories/agentchat/internal/store/hub.go, wired in
// repositories.Container), so the single source of lifecycle frames is the event
// stream. The usecase's job ends at issuing the command that emits the event.
type Usecase struct {
	chats    agentchat.EventStore
	registry *engineagent.Registry
	term     TerminalCommander
	ws       WorkspaceReader
}

// New builds a Usecase from the agentchat EventStore, reducer registry, and
// seams. There is no per-segment serialization mutex any more: the asynx
// write-path (id,version) optimistic concurrency (sendWithOCC in the
// repository) is the concurrency control, replacing the retired keyed_mutex.
func New(
	chats agentchat.EventStore,
	registry *engineagent.Registry,
	term TerminalCommander,
	ws WorkspaceReader,
) *Usecase {
	return &Usecase{
		chats:    chats,
		registry: registry,
		term:     term,
		ws:       ws,
	}
}

// SpawnChat creates a fresh AgentChat and its first AgentSegment, launching the
// provider's vendor CLI in a PTY. The returned segID is also the segment's
// CrowbarSegmentID and terminal session id.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, segID string, err error) {
	chatID = uuid.NewString()
	segID, err = u.spawnSegment(ctx, chatID, workspaceID, providerID, nil, "", true, true)
	if err != nil {
		return "", "", err
	}
	return chatID, segID, nil
}

// RenameChat sets a chat's title under user>agent>derived precedence:
//
//	source "derived": set only if the title is currently empty (first-prompt fallback).
//	source "agent":   set unless the title is user-locked (agent may upgrade a derived title).
//	source "user"/"": set unconditionally AND lock (a manual rename wins and sticks).
//
// The empty-title-is-a-no-op and derived-only-if-empty gates live here (the
// SetTitle command only enforces the locked-vs-user rule); an empty title is
// always a no-op. A successful change emits a title_set event, which the hub
// projection fans out as the lifecycle frame — the usecase no longer broadcasts.
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

// PurgeChat hard-deletes chatID via asynx Forget (Task 5): it erases the
// aggregate outright — its event log AND read-model row are both gone, so a
// subsequent GetChat/ListChats/ListByWorkspace genuinely reports not found.
// If the chat's active segment still has a live vendor-CLI PTY, that process is
// terminated FIRST so a chat delete doesn't orphan a running CLI — but
// BEST-EFFORT: a terminate failure is logged and the purge proceeds anyway.
// Wedging the purge on a terminate error (the user could then never remove the
// chat) is a worse outcome than an orphaned PTY, whose live turn state the boot
// reconcile (ReconcileOnBoot) repairs on the next restart — ending the dead
// segment and reaping its per-spawn tmp dir — regardless. ErrSessionNotFound
// (the CLI already exited) is not even logged. This is the standalone
// (single-chat) counterpart to the workspace-delete cascade's forgetAgentChats
// (repositories.Container), which Forgets every chat anchored to a workspace,
// with the same best-effort teardown, when the workspace itself is deleted.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	// Unbind the chat's segments from the registry BEFORE tearing down the PTY.
	// The teardown fires reconcileSegmentExit asynchronously (terminal-engine reap
	// goroutine); its FIRST guard is the in-memory registry (ChatFor). Unbinding
	// here makes that guard fail closed (ChatFor → !ok → no-op), so the teardown
	// can never emit a segment_ended event for this deleted chat. Its fallback
	// GetChat guard is NOT enough on its own: asynx delivers bus events on a
	// goroutine per handler, so onForget's read-model row-delete races
	// reconcile's EndSegment Save — a fast-exiting CLI re-Saves the row after
	// onForget deleted it, RESURRECTING the chat as a zombie with an "ended"
	// segment (found via live daemon testing; the async race is invisible to a
	// slow-exiting real CLI but not to an instant stub exit).
	u.registry.ForgetChat(chatID)
	if seg, ok := segmentByID(chat, chat.ActiveSegmentID); ok && seg.TerminalSessionID != "" {
		if err := u.term.TerminateGraceful(ctx, seg.TerminalSessionID); err != nil &&
			!errors.Is(err, engineterminal.ErrSessionNotFound) {
			slog.WarnContext(ctx, "agent: purge chat: terminate active segment (best-effort, continuing)",
				"chat_id", chatID, "segment_id", seg.ID, "terminal_session_id", seg.TerminalSessionID, "err", err)
		}
	}
	if err := u.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}
	// Reap the chat's on-disk footprint (the handoff ledger + any residual
	// per-segment tmp dir) now the aggregate is Forgotten. Unlike the
	// workspace-delete cascade — which rm's the whole workspace root — a
	// standalone hard delete would otherwise leave this chat's PLAINTEXT ledger
	// behind (Important-2). The removal is routed through RemoveUnderHome, which
	// re-asserts the target is strictly under crowbar home, so even a poisoned
	// chats dir can never reach the user's real repository. Best-effort: a lookup
	// or rm failure is logged, never returned — the aggregate is already gone and
	// a leftover dir is a far smaller harm than failing a delete the user asked for.
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

// ForgetChatRegistry unbinds chatID's segments from the in-memory context-move
// registry. The workspace-delete cascade (repositories.Container.forgetAgentChats)
// calls it — through a settable seam it can't construct directly — before tearing
// down each chat's PTY, for the SAME reason PurgeChat unbinds inline: so the
// teardown's async reconcileSegmentExit no-ops at its ChatFor guard instead of
// racing onForget's read-model row-delete and resurrecting the Forgotten chat as
// a zombie row (see PurgeChat + Registry.ForgetChat).
func (u *Usecase) ForgetChatRegistry(chatID string) {
	u.registry.ForgetChat(chatID)
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

// spawnSegment is the single spawn seam and the single owner of a chat's
// ActiveSegmentID. It does ALL spawn IO first — bind the segment to its chat in
// the reducer, resolve the descriptor, render the per-spawn tmp dir + hook
// config, build the spawn plan, and launch the PTY — and only THEN issues the
// persistence command: Create for a genuine fresh chat (create=true), or
// OpenSegment for a switch-in / resume on an existing chat (create=false). Both
// SpawnChat and SwitchProvider go through it so ActiveSegmentID is never left
// unset. injectTitle is true only for a fresh-chat spawn: it injects the
// configured title instruction as a true system-prompt document via the
// descriptor's system_prompt_inject steps, instead of the (here empty) handoff.
//
// IO-before-command ordering is load-bearing for the concurrent-switch rule: a
// pure command cannot spawn a process, so the CLI is already live when
// OpenSegment runs. If OpenSegment loses a version race (an active segment
// already exists — asynx ErrValidation) or fails for any other reason, the
// just-spawned CLI is torn down (TerminateGraceful) so no orphan process leaks.
func (u *Usecase) spawnSegment(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	extraSteps []engineagent.InjectStep,
	handoff string,
	injectTitle bool,
	create bool,
) (string, error) {
	segID := uuid.NewString()
	// Bind the segment to its chat BEFORE the PTY starts, so a hook fired the
	// instant the CLI comes up already routes to this chat (ChatFor). A stale
	// binding left by a later spawn/persist failure is harmless: segID is a
	// fresh uuid never reused, so no hook can match a segment that was never
	// persisted.
	u.registry.BindSegment(segID, chatID)

	crowbarHome, projectID, repoID, worktree, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: worktree dir: %w", err)
	}

	// The chats dir is resolved separately from the worktree/Cwd: for a home-kind
	// (adopted checkout) workspace the worktree is the user's REAL dir outside
	// home, so chat state (this tmp dir, the ledger) reroots under crowbar home
	// while the CLI still runs with Cwd = worktree (Task 7).
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: chats dir: %w", err)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, providerID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: resolve descriptor: %w", err)
	}

	// Under the workspace's chats dir (always beneath crowbar home), keyed by
	// chatID+segID+providerID so it is deterministic per spawn. This dir holds
	// the rendered hook config and, for codex, a COPY of ~/.codex/auth.json (a
	// credential) — it must survive for the whole life of the spawned CLI, so
	// it is removed via onExit below (on PTY session end), never eagerly after
	// spawn.
	tmpDir := worktreepath.SegmentDir(chatsDir, chatID, segID, providerID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return "", fmt.Errorf("agent: spawn segment: mkdir tmp: %w", err)
	}

	tctx := engineagent.TemplateCtx{
		Tmp:         tmpDir,
		Cwd:         worktree,
		CrowbarHook: u.crowbarHookPath(crowbarHome),
		Segid:       segID,
		Provider:    providerID,
		Chatid:      chatID,
		ProjectID:   projectID,
		RepoID:      repoID,
		WorkspaceID: workspaceID,
	}
	steps := extraSteps
	if injectTitle {
		// Fresh chat: the injected document is the title instruction (from
		// config), delivered through the descriptor's system_prompt_inject
		// mechanism — a true per-invocation system prompt, NOT handoff_inject
		// (which for codex is a positional arg that would otherwise hijack its
		// initial user turn; see descriptor.go's SystemPromptInject doc).
		tctx.SystemPrompt = engineagent.Expand(config.GetPrompts().TitleInstruction, tctx)
		steps = append(steps, descriptor.SystemPromptInject...)
	} else {
		tctx.Handoff = handoff
	}
	plan, err := engineagent.BuildSpawnPlan(descriptor, tctx, os.Environ(), steps)
	if err != nil {
		return "", fmt.Errorf("agent: spawn segment: build spawn plan: %w", err)
	}

	argv := append([]string{descriptor.Spawn.Cmd}, plan.Argv...)

	termSessID, err := u.term.CreateCommand(ctx, workspaceID, worktree, argv, plan.Env,
		u.onSegmentExit(crowbarHome, segID, tmpDir))
	if err != nil {
		// CreateCommand never got far enough to register onExit (which is what
		// rm's the tmp dir on a clean exit) — clean up here so a spawn failure
		// doesn't leak the segment's tmp dir. Guarded by crowbarHome so a poisoned
		// chats dir can never make this rm escape the user's real filesystem.
		RemoveUnderHome(ctx, crowbarHome, tmpDir)
		return "", fmt.Errorf("agent: spawn segment: create command: %w", err)
	}

	now := time.Now()
	if create {
		_, err = u.chats.Create(ctx, agentchat.CreateInput{
			ID:               chatID,
			WorkspaceID:      workspaceID,
			SegmentID:        segID,
			CrowbarSegmentID: segID,
			ProviderID:       providerID,
			TerminalSession:  termSessID,
			Now:              now,
		})
	} else {
		_, err = u.chats.OpenSegment(ctx, agentchat.OpenSegmentInput{
			ChatID:           chatID,
			SegmentID:        segID,
			CrowbarSegmentID: segID,
			ProviderID:       providerID,
			TerminalSession:  termSessID,
			Now:              now,
		})
	}
	if err != nil {
		// The CLI is already live but its segment could not be persisted — tear
		// it down so we never leak an orphan process. The headline case is
		// OpenSegment losing a concurrent-switch version race (asynx
		// ErrValidation: an active segment already exists); every other error
		// leaves the same orphan, so the teardown is unconditional. The original
		// error is returned wrapped, so ErrValidation still classifies as a
		// conflict upstream. TerminateGraceful's own "session already gone" is
		// harmless here and ignored.
		if termErr := u.term.TerminateGraceful(ctx, termSessID); termErr != nil &&
			!errors.Is(termErr, engineterminal.ErrSessionNotFound) {
			slog.WarnContext(ctx, "agent: spawn segment: teardown after persist failure",
				"chat_id", chatID, "segment_id", segID, "terminal_session_id", termSessID, "err", termErr)
		}
		return "", fmt.Errorf("agent: spawn segment: persist segment: %w", err)
	}
	return segID, nil
}

// RemoveUnderHome is the SINGLE guarded os.RemoveAll every agent-path filesystem
// removal routes through (Task 7 safety hardening). It removes target ONLY when
// target is provably strictly under crowbar home — worktreepath.UnderHome, the
// exact strict-prefix check the worktree removers (worktreeRemover/bootSweepPurge)
// re-assert on their own root. AgentChatsDir already reroots chat state under
// home, but filepath.Join CLEANS "..", so a crafted repo remote slug
// (host/owner/../../..) could in principle collapse a derived path OUTSIDE home;
// this re-asserts the invariant AT the rm so no agent removal can EVER reach the
// user's real filesystem, even if a caller is handed a poisoned chats dir. A
// target not under home (including a blank/unresolvable home) is logged and
// skipped, never removed — fail-closed. Callers are all best-effort, so a plain
// rm error is logged, not returned.
//
// Exported so the workspace-delete cascade's on-disk reap seam
// (app.reapAgentChatFiles, wired into repositories.Container.ReapChatFiles) can
// route through the exact SAME guard PurgeChat uses, rather than a package
// outside agent reimplementing the check.
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

// onSegmentExit builds the CreateCommand onExit callback for segID: it
// releases the per-spawn tmp dir (through RemoveUnderHome, so a poisoned path can
// never escape crowbar home) and then reconciles live turn state for a CLI
// process death that no event ever recorded — a daemon-observed crash/self-exit,
// as opposed to a clean provider switch or context-move, both of which already
// end the segment themselves via an explicit EndSegment before or independently
// of the process actually dying. home is captured at spawn time (the same home
// tmpDir was created under) so the guard needs no post-hoc lookup that a since
// deleted workspace could fail. It runs on a background context: the terminal
// engine invokes onExit from its own reap goroutine, well after any request
// context that spawned this segment could have been cancelled.
func (u *Usecase) onSegmentExit(home, segID, tmpDir string) func() {
	return func() {
		RemoveUnderHome(context.Background(), home, tmpDir)
		u.reconcileSegmentExit(context.Background(), segID)
	}
}

// reconcileSegmentExit ends segID's chat segment and, if the chat was still
// Working, stops the turn — but ONLY when segID is still that chat's active
// segment at the moment this runs. This is what keeps a process-death
// reconcile from fighting an explicit provider switch: SwitchProvider's own
// EndSegment call (and the context-move reducer's moveToNewChat/
// moveToKnownChat) always run first when they are the ones tearing the
// segment down, so by the time this fires for that same segment it is no
// longer "active" and activeSegment returns false — a deliberate no-op, never
// a double-end, and never a reconcile of whatever segment has since taken
// over as active in that chat. Errors are logged, not returned: onExit runs
// off the terminal engine's reap goroutine with no caller to hand an error to.
func (u *Usecase) reconcileSegmentExit(ctx context.Context, segID string) {
	chatID, ok := u.registry.ChatFor(segID)
	if !ok {
		// The registry never learned this segment (should not happen — it is
		// bound before spawn), so there is no chat to reconcile against.
		return
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		if !errors.Is(err, agentchat.ErrNotFound) {
			slog.WarnContext(ctx, "agent: reconcile segment exit: get chat",
				"chat_id", chatID, "segment_id", segID, "err", err)
		}
		return
	}
	seg, ok := activeSegment(chat, segID)
	if !ok {
		// Already ended by an explicit path (provider switch / context-move) —
		// nothing left to reconcile for this segment.
		return
	}
	// Pass seg.ID (the aggregate segment id of the still-active segment for this
	// live process), not segID (the crowbarSegID bound at spawn): after a
	// context-move the two diverge, and EndSegment's guard matches on the
	// aggregate segment id (ActiveSegmentID).
	if err := u.endSegmentAndMaybeStopTurn(ctx, chatID, seg.ID); err != nil {
		slog.WarnContext(ctx, "agent: reconcile segment exit: end segment",
			"chat_id", chatID, "segment_id", segID, "err", err)
	}
}

// endSegmentAndMaybeStopTurn ends chatID's segmentID (the segment the caller
// observed as active) and, only if the chat was still Working, stops the turn
// too — shared by the runtime process-exit reconcile (reconcileSegmentExit) and
// ReconcileOnBoot so both apply the exact same "end, then stop the turn only if
// it was actually running" rule. A StopTurn is skipped when the chat was not
// Working so a dead segment on a chat that already finished its turn doesn't
// emit a redundant turn_stopped event.
//
// EndSegment is segment-scoped: it ends segmentID only while it is still THE
// active segment, otherwise it is a no-op (a concurrent switch already replaced
// it). When it no-ops, ActiveSegmentID stays pointing at the NEW segment, so
// the follow-on StopTurn is suppressed too — a stale reconcile must never stop
// the brand-new segment's turn.
func (u *Usecase) endSegmentAndMaybeStopTurn(ctx context.Context, chatID, segmentID string) error {
	now := time.Now()
	chat, err := u.chats.EndSegment(ctx, chatID, segmentID, now)
	if err != nil {
		return fmt.Errorf("end segment: %w", err)
	}
	if chat.ActiveSegmentID != "" {
		// EndSegment was a no-op: segmentID is no longer the active segment (a
		// concurrent switch replaced it). Leave the new segment's turn alone.
		return nil
	}
	if !chat.Working {
		return nil
	}
	if _, err := u.chats.StopTurn(ctx, chatID, now); err != nil {
		return fmt.Errorf("stop turn: %w", err)
	}
	return nil
}

func (u *Usecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}

// IngestHook maps an incoming vendor hook to a canonical event, runs the
// context-move reducer on session_start, and appends a conversation turn to the
// chat's ledger on user_prompt / turn_stop. It also drives the aggregate's live
// turn state: a user_prompt opens the turn (StartTurn → Working) and a turn_stop
// closes it (StopTurn → idle), so domain.AgentChat.Working reflects reality and
// the boot reconcile's interrupted-turn repair is reachable. Routing is by
// crowbarSegID via the reducer's segment→chat index (Registry.ChatFor) — the
// in-memory successor to the retired GetActiveSegmentByCrowbarID lookup. An
// unknown crowbarSegID (no live segment), a chat with no matching active
// segment, or a malformed payload is ignored, never an error — a hook must never
// break the vendor CLI's turn.
func (u *Usecase) IngestHook(
	ctx context.Context,
	crowbarSegID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	chatID, ok := u.registry.ChatFor(crowbarSegID)
	if !ok {
		return nil
	}

	chat, err := u.chats.GetChat(ctx, chatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: ingest hook: chat: %w", err)
	}

	seg, ok := activeSegment(chat, crowbarSegID)
	if !ok {
		return nil
	}

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The active segment is the source of truth for which provider spawned this
	// CLI. The hook's self-reported provider is only a guard against a
	// mis-authored descriptor.
	if provider != "" && provider != seg.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "segment_provider", seg.ProviderID, "segment_id", crowbarSegID)
	}

	descriptor, err := engineagent.ResolveDescriptor(crowbarHome, seg.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	payload, err := descriptor.ParsePayload(rawPayload)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: parse payload", "err", err, "segment_id", crowbarSegID)
		return nil
	}

	ev, _ := descriptor.MapHook(canonicalEvent, payload)

	switch ev.Kind {
	case "session_start":
		return u.handleSessionStart(ctx, crowbarSegID, chat, seg, ev)
	case "user_prompt":
		if err := u.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
			slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
		}
		// A user prompt opens the turn: mark the chat Working so the read model
		// (and the boot reconcile's interrupted-turn branch) see a live turn.
		if _, err := u.chats.StartTurn(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		return u.appendTurn(ctx, seg, chat, "user", ev.Message)
	case "turn_stop":
		// The turn ended: clear Working. Issued before the ledger append so the
		// live-state event lands even when the assistant message is empty (an
		// empty message is a ledger no-op, not a turn-state no-op).
		if _, err := u.chats.StopTurn(ctx, chat.ID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: stop turn: %w", err)
		}
		return u.appendTurn(ctx, seg, chat, "assistant", ev.Message)
	}
	return nil
}

// activeSegment returns the single active segment for a live process
// (its crowbarSegID) within a chat aggregate. The command layer's ≤1-active
// invariant guarantees at most one match.
func activeSegment(chat domain.AgentChat, crowbarSegID string) (domain.AgentSegment, bool) {
	for _, s := range chat.Segments {
		if s.CrowbarSegmentID == crowbarSegID && s.Status == "active" {
			return s, true
		}
	}
	return domain.AgentSegment{}, false
}

// handleSessionStart runs the spec §7 context-move reducer and maps its outcome
// to commands. oldChat/oldSeg are the chat currently hosting the live process
// and its active segment (read before the reducer decides). The reducer may
// keep the process where it is (bound), move it to a brand-new chat
// (registered), or move it into a known chat it once inhabited (focus).
func (u *Usecase) handleSessionStart(
	ctx context.Context,
	crowbarSegID string,
	oldChat domain.AgentChat,
	oldSeg domain.AgentSegment,
	ev engineagent.CanonicalEvent,
) error {
	out := u.registry.OnSessionStart(crowbarSegID, ev.SessionID, uuid.NewString)

	switch out.Kind {
	case "bound":
		return u.bindSession(ctx, out.ChatID, crowbarSegID, oldSeg, ev)
	case "registered":
		return u.moveToNewChat(ctx, crowbarSegID, oldChat, oldSeg, out, ev)
	case "focus":
		return u.moveToKnownChat(ctx, crowbarSegID, oldChat, oldSeg, out, ev)
	}
	return nil
}

// bindSession records the provider's native session id on the segment's first
// session_start. It never overwrites an already-bound id (the reducer only
// returns "bound" for a segment whose id it has not seen, so a set id here is a
// pre-existing binding to preserve).
func (u *Usecase) bindSession(
	ctx context.Context,
	chatID string,
	crowbarSegID string,
	oldSeg domain.AgentSegment,
	ev engineagent.CanonicalEvent,
) error {
	if oldSeg.ProviderSessionID != "" {
		return nil
	}
	if _, err := u.chats.BindSession(ctx, chatID, crowbarSegID, ev.SessionID); err != nil {
		return fmt.Errorf("agent: ingest hook: bound: bind session: %w", err)
	}
	return nil
}

// moveToNewChat handles the reducer's "registered" outcome: the live process
// reported a brand-new (unknown) session id, so it has moved into a fresh chat.
// End the old chat's active segment (which also clears its now-stale
// ActiveSegmentID), create the new chat carrying the SAME crowbarSegID and
// terminal session, and bind the new native session id onto it.
func (u *Usecase) moveToNewChat(
	ctx context.Context,
	crowbarSegID string,
	oldChat domain.AgentChat,
	oldSeg domain.AgentSegment,
	out engineagent.Outcome,
	ev engineagent.CanonicalEvent,
) error {
	now := time.Now()
	if _, err := u.chats.EndSegment(ctx, oldChat.ID, oldSeg.ID, now); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: end old segment: %w", err)
	}
	if _, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:               out.ChatID,
		WorkspaceID:      oldChat.WorkspaceID,
		SegmentID:        uuid.NewString(),
		CrowbarSegmentID: crowbarSegID,
		ProviderID:       oldSeg.ProviderID,
		TerminalSession:  oldSeg.TerminalSessionID,
		Now:              now,
	}); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: create new chat: %w", err)
	}
	if _, err := u.chats.BindSession(ctx, out.ChatID, crowbarSegID, ev.SessionID); err != nil {
		return fmt.Errorf("agent: ingest hook: registered: bind session: %w", err)
	}
	return nil
}

// moveToKnownChat handles the reducer's "focus" outcome: the live process
// reported a session id already known to belong to another (existing) chat, so
// it has moved back into that chat. End the old chat's active segment (clearing
// its stale ActiveSegmentID) and open a new active segment on the focused chat
// carrying the same crowbarSegID and terminal session, then bind the session.
// EndSegment runs unconditionally: when the process focuses back into the very
// chat it is already in, ending the current active segment first is exactly
// what lets OpenSegment (which rejects a chat that still has an active segment)
// succeed.
func (u *Usecase) moveToKnownChat(
	ctx context.Context,
	crowbarSegID string,
	oldChat domain.AgentChat,
	oldSeg domain.AgentSegment,
	out engineagent.Outcome,
	ev engineagent.CanonicalEvent,
) error {
	now := time.Now()
	if _, err := u.chats.EndSegment(ctx, oldChat.ID, oldSeg.ID, now); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: end old segment: %w", err)
	}
	if _, err := u.chats.OpenSegment(ctx, agentchat.OpenSegmentInput{
		ChatID:           out.ChatID,
		SegmentID:        uuid.NewString(),
		CrowbarSegmentID: crowbarSegID,
		ProviderID:       oldSeg.ProviderID,
		TerminalSession:  oldSeg.TerminalSessionID,
		Now:              now,
	}); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: open segment: %w", err)
	}
	if _, err := u.chats.BindSession(ctx, out.ChatID, crowbarSegID, ev.SessionID); err != nil {
		return fmt.Errorf("agent: ingest hook: focus: bind session: %w", err)
	}
	return nil
}

// appendTurn records one conversation turn (user or assistant) into the chat's
// ledger. Empty text is a no-op. The turn's lifecycle frame is emitted by the
// StartTurn/StopTurn events the caller issues (fanned out by the hub
// projection), not from here — the ledger is a content log, not an aggregate.
//
// It resolves the chats dir itself (after the empty-text short-circuit) rather
// than taking a worktree path: the ledger always lives under the workspace's
// chats dir, which for a home-kind / adopted-checkout workspace is rerooted under
// crowbar home, NOT beside the user's real worktree (Task 7).
func (u *Usecase) appendTurn(
	ctx context.Context,
	seg domain.AgentSegment,
	chat domain.AgentChat,
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
	if _, err := led.AppendTurn(role, seg.ProviderID, time.Now(), text); err != nil {
		return fmt.Errorf("agent: ingest hook: ledger append: %w", err)
	}
	return nil
}

// AssembleHandoff resolves chatID's ledger directory, reads every entry, and
// wraps them in a legible preamble/footer so a freshly spawned provider CLI
// can be handed the prior context. Returns "" (not an error) when the ledger
// has no entries yet.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: chat: %w", err)
	}

	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: chats dir: %w", err)
	}

	dir := worktreepath.AgentLedgerDir(chatsDir, chat.ID)
	led, err := ledger.Open(dir)
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: ledger open: %w", err)
	}

	blob, err := led.RenderConversation()
	if err != nil {
		return "", fmt.Errorf("agent: assemble handoff: ledger read all: %w", err)
	}
	if len(blob) == 0 {
		return "", nil
	}

	return strings.ReplaceAll(config.GetPrompts().HandoffWrapper, "{conversation}", string(blob)), nil
}

// SwitchProvider is the headline provider-switch: it terminates chatID's active
// provider CLI, assembles a handoff from the ledger, and spawns
// targetProviderID as a NEW segment in the SAME chat with the handoff injected.
// If a prior segment for targetProviderID already carries a native
// ProviderSessionID (a switch-back), the target CLI is also resumed into that
// session via the descriptor's session.resume; otherwise (a forward switch) it
// receives only the handoff. Reuses spawnSegment (create=false), the single
// owner of ActiveSegmentID, so segment creation — and the concurrent-switch
// orphan teardown — is never duplicated here.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: chat: %w", err)
	}

	oldSeg, ok := segmentByID(chat, chat.ActiveSegmentID)
	if !ok {
		return "", fmt.Errorf("agent: switch provider: active segment: %w", agentchat.ErrNotFound)
	}

	// Read-before-terminate: the ledger is built from hooks (appended on each
	// turn_stop/user_prompt hook) and is already on disk, so assembling the
	// handoff does not depend on the outgoing CLI still being alive. A
	// failure here must abort the switch (return before terminate) rather
	// than silently proceed with an EMPTY handoff — nothing destructive has
	// happened yet, so aborting leaves the chat exactly as it was.
	handoff, err := u.AssembleHandoff(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
	}

	// The outgoing CLI is quit gracefully (spec §8: clean-exit SIGTERM, never
	// SIGKILL) so a well-behaved CLI — Claude Code in particular — gets the
	// chance to flush its native transcript before it dies; a hard kill can
	// lose the outgoing CLI's last pre-switch turn. Applies uniformly to
	// every provider (Codex tolerates it too), no per-provider branching.
	//
	// A failed terminate is surfaced, not swallowed: if the outgoing CLI's
	// terminal session still exists but could not be terminated, proceeding
	// would spawn a SECOND live CLI into the same worktree while the store marks
	// only the new one active — the exact "two live CLIs" hazard this guards
	// against. The one error TerminateGraceful can return today
	// (registry.ErrSessionNotFound, exported as terminal.ErrSessionNotFound)
	// means the session is already gone (process previously exited/reaped) —
	// safe, even correct, to continue: the alternative would trap a chat
	// unable to ever switch again once its terminal session ends on its own.
	if err := u.term.TerminateGraceful(ctx, oldSeg.TerminalSessionID); err != nil {
		if !errors.Is(err, engineterminal.ErrSessionNotFound) {
			return "", fmt.Errorf("agent: switch provider: terminate outgoing terminal: %w", err)
		}
		slog.WarnContext(ctx, "agent: switch provider: outgoing terminal session already gone before terminate; continuing switch",
			"chat_id", chatID, "segment_id", oldSeg.ID, "terminal_session_id", oldSeg.TerminalSessionID, "err", err)
	}

	// End the outgoing segment BEFORE spawning the target: OpenSegment (inside
	// spawnSegment) rejects a chat that still has an active segment, so the
	// active segment must be cleared first. Scoped to oldSeg.ID so a concurrent
	// reconcile/switch racing this same chat can't make us end a segment other
	// than the one we read as active.
	if _, err := u.chats.EndSegment(ctx, chatID, oldSeg.ID, time.Now()); err != nil {
		return "", fmt.Errorf("agent: switch provider: end old segment: %w", err)
	}

	// chat.Segments is in append (start) order, so the LAST match while scanning
	// forward is the most recent prior segment for the target provider. This is
	// the switch-back detector: a target that already ran in this chat and bound
	// a native session id is resumed into it; a forward switch finds none.
	var priorSessionID string
	for _, s := range chat.Segments {
		if s.ProviderID == targetProviderID && s.ProviderSessionID != "" {
			priorSessionID = s.ProviderSessionID
		}
	}

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: worktree dir: %w", err)
	}
	d, err := engineagent.ResolveDescriptor(crowbarHome, targetProviderID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
	}

	// Resume arg must be split into separate argv tokens: exec.Command does
	// NOT split a string on whitespace, so a whole "--resume {id}" template
	// handed to a single pass_arg would become one literal argument.
	var resumeSteps []engineagent.InjectStep
	if priorSessionID != "" && d.Session.Resume != nil && d.Session.Resume.Arg != "" {
		resumeCtx := engineagent.TemplateCtx{ID: priorSessionID}
		for _, tok := range strings.Fields(engineagent.Expand(d.Session.Resume.Arg, resumeCtx)) {
			resumeSteps = append(resumeSteps, engineagent.InjectStep{
				Verb: "pass_arg",
				Args: map[string]any{"positional": tok},
			})
		}
	}
	// Resume goes first so codex's `resume <id>` subcommand precedes the
	// positional handoff; order is irrelevant for claude's flag pair.
	extraSteps := append(resumeSteps, d.HandoffInject...)

	newSegID, err := u.spawnSegment(ctx, chatID, chat.WorkspaceID, targetProviderID, extraSteps, handoff, false, false)
	if err != nil {
		return "", err
	}
	return newSegID, nil
}

// segmentByID returns the segment with the given id from a chat aggregate.
func segmentByID(chat domain.AgentChat, id string) (domain.AgentSegment, bool) {
	for _, s := range chat.Segments {
		if s.ID == id {
			return s, true
		}
	}
	return domain.AgentSegment{}, false
}

// ListChats returns every AgentChat.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.chats.ListChats(ctx)
}

// ListChatsByWorkspace returns every AgentChat anchored to workspaceID
// (Task 3: backs the workspace-scoped List REST route).
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

// SegmentsFor returns every AgentSegment belonging to a chat, oldest first
// (segments are embedded in the aggregate in append/start order).
func (u *Usecase) SegmentsFor(
	ctx context.Context,
	chatID string,
) ([]domain.AgentSegment, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	return chat.Segments, nil
}

// SeedRegistry rehydrates the reducer's known-session index from persisted
// chats at startup, so a resumed process that /resumes into a pre-restart chat
// is recognized as "focus" rather than "registered". Segments are embedded, so
// this scans every live chat's segments rather than a flat segment table.
func (u *Usecase) SeedRegistry(
	ctx context.Context,
) error {
	chats, err := u.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: seed registry: list chats: %w", err)
	}
	for _, chat := range chats {
		for _, seg := range chat.Segments {
			if seg.ProviderSessionID == "" {
				continue
			}
			u.registry.Seed(seg.ProviderSessionID, chat.ID)
		}
	}
	return nil
}

// ReconcileOnBoot repairs live turn state a daemon crash can leave stale: no
// event ever records "the CLI process died," so a chat's ActiveSegmentID /
// Working can survive a restart pointing at a terminal session that no
// longer exists (see domain.AgentChat's doc comment on Working). It lists
// every live chat and, for one whose active segment's TerminalSessionID is
// NOT a live terminal session (per the injected TerminalCommander.
// SessionExists), ends that segment and — if the chat was still Working —
// stops the turn, via the same endSegmentAndMaybeStopTurn rule the runtime
// onExit reconcile uses. A chat whose active segment's terminal session IS
// still live (including a restored placeholder) is left untouched.
//
// This deliberately does NOT live in a repository reactor: SessionExists is a
// terminal-engine concern the repository layer cannot reach, so it lives here
// as a usecase method instead — a sibling to SeedRegistry, not a replacement
// for it. The caller must run it AFTER the terminal engine has repopulated
// its registry from persisted sessions (startRestoreTerminalSessions) so a
// session merely reloaded as a suspended placeholder still reads alive and is
// never wrongly reconciled. Best-effort per chat: one chat's reconcile
// failure is logged and does not stop the rest from being reconciled.
func (u *Usecase) ReconcileOnBoot(
	ctx context.Context,
) error {
	chats, err := u.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: reconcile on boot: list chats: %w", err)
	}
	for _, chat := range chats {
		seg, ok := segmentByID(chat, chat.ActiveSegmentID)
		if !ok || seg.Status != "active" {
			continue
		}
		if u.term.SessionExists(ctx, seg.TerminalSessionID) {
			continue
		}
		if err := u.endSegmentAndMaybeStopTurn(ctx, chat.ID, seg.ID); err != nil {
			slog.WarnContext(ctx, "agent: reconcile on boot: end segment",
				"chat_id", chat.ID, "segment_id", seg.ID, "terminal_session_id", seg.TerminalSessionID, "err", err)
		}
		// This active segment's PTY died with the daemon (SessionExists==false),
		// so its per-spawn tmp dir (rendered hook config + any codex auth.json
		// COPY) is a crash orphan: the onExit cleanup that removes it on a clean
		// exit never fired. It is the ONLY orphan class under the workspace-root
		// layout — a clean exit is ephemeral (onSegmentExit rm's it) and a
		// chat/workspace delete rm's the whole workspace root — so reaping it
		// here is the targeted successor to the retired global agent-tmp sweep
		// (which blindly wiped <home>/agent-tmp; that dir no longer exists).
		u.reapCrashOrphanSegmentTmp(ctx, chat, seg)
	}
	return nil
}

// reapCrashOrphanSegmentTmp removes the per-spawn tmp dir of a segment whose PTY
// died with the daemon, so a codex auth.json copy and the rendered hook config
// don't linger under the workspace root after a crash. Best-effort: a worktree
// lookup or rm failure is logged, never fatal — a leftover tmp dir is harmless
// (it is overwritten by segId+provider on any future spawn and swept with the
// workspace root on delete). It is keyed by the CURRENT aggregate segment id;
// for the common (non-context-moved) segment that equals the spawn-time key the
// dir was created under, so the path resolves exactly. A segment that was
// context-moved before the crash carries a diverged aggregate id AND a diverged
// hosting chat, so its original tmp dir (created under the pre-move chat/segid)
// is not reached here — an accepted, rare corner that leaves at most one small
// dir, strictly better than the zero cleanup the old layout's removed sweep left.
func (u *Usecase) reapCrashOrphanSegmentTmp(ctx context.Context, chat domain.AgentChat, seg domain.AgentSegment) {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: reconcile on boot: reap segment tmp: chats dir",
			"chat_id", chat.ID, "segment_id", seg.ID, "err", err)
		return
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: reconcile on boot: reap segment tmp: home",
			"chat_id", chat.ID, "segment_id", seg.ID, "err", err)
		return
	}
	// RemoveUnderHome re-asserts the target is strictly under crowbar home, so even
	// a poisoned chats dir can never make this reap escape the user's filesystem.
	RemoveUnderHome(ctx, home, worktreepath.SegmentDir(chatsDir, chat.ID, seg.ID, seg.ProviderID))
}
