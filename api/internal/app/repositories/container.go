package repositories

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	ReviewThread reviewthread.ReviewThread
	// AgentChat is the asynx-backed EventStore: its store/hub projections are
	// live on axAgentChat and the agent usecase sends every AgentChat mutation
	// through it (the gorm-backed store was retired in the Task 10 cutover).
	AgentChat agentchat.EventStore
	hub       hub.WebSocketHub
	git       wsusecase.MergeConflictChecker
	// terminateSession is the terminal-engine seam the workspace-delete cascade
	// (forgetAgentChats, Task 12) uses to kill a chat's live vendor-CLI PTY
	// before Forgetting it. It is injected from the app layer (which owns the
	// terminal engine) as a plain func, exactly like worktreeRemover is
	// injected as a plain func rather than an fs/git type — this package holds
	// no terminal-engine dependency of its own. Nil is safe: it just skips PTY
	// teardown (tests that don't exercise it, or a build with no terminal
	// engine wired).
	terminateSession func(ctx context.Context, sessionID string) error
	// ReapChatFiles is the workspace-delete cascade's on-disk reap seam
	// (forgetAgentChats): given a forgotten chat's (workspaceID, chatID), it
	// removes that chat's own <chatsDir>/<chatID> directory (the handoff ledger +
	// any residual per-segment tmp dir) — closing the gap where Forgetting the
	// event-sourced aggregate left its PLAINTEXT on-disk footprint behind under
	// .crowbar. Unlike terminateSession (a New(...) constructor parameter), this
	// is a settable field the app layer assigns AFTER construction: the real
	// implementation (app.reapAgentChatFiles) reuses the SAME agent.WorkspaceReader
	// (AgentChatsDir + WorktreeDir) and agent.RemoveUnderHome guard the standalone
	// PurgeChat path already routes through, and that reader is built from
	// repos.Workspace — which does not exist until repositories.New returns — so
	// it cannot be threaded in as a constructor argument without a genuine
	// construction cycle (usecases.New itself takes *repositories.Container).
	// Exported (not routed through a setter) so tests can inject a fake exactly
	// like they already do for c.Workspace. Nil is safe: reaping is skipped.
	ReapChatFiles func(ctx context.Context, wsID, chatID string) error
	// ForgetChatRegistry is the workspace-delete cascade's registry-unbind seam
	// (forgetAgentChats): it removes a chat's segment->chat bindings from the agent
	// usecase's in-memory context-move registry BEFORE that chat's PTY is torn
	// down, so the teardown's async reconcile (reconcileSegmentExit) no-ops at its
	// ChatFor guard instead of racing onForget's row-delete and resurrecting the
	// chat as a zombie read row (the standalone-delete counterpart is PurgeChat's
	// own u.registry.ForgetChat call). Same construction-order rationale as
	// ReapChatFiles — it depends on the agent usecase, built after repositories.New
	// — so the app layer assigns it after construction. Nil is safe: the unbind is
	// skipped (a build/test with no agent registry wired).
	ForgetChatRegistry func(chatID string)
	// axWorkspace/axReviewThread/axAgentChat are the per-type asynx instances,
	// retained so WaitQuiescent can drain their dispatch queues + projection
	// handlers — the deterministic read-your-writes barrier for tests (no
	// polling, no timeouts).
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.AgentChat]
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int

	// agentWorking maps a workspace id to the set of its agent chats currently
	// mid-turn (00 agentic-engine spec §7.4). It is the agent-turn counterpart to
	// inflight: enrichFrame ORs it into the derived Working overlay, and the
	// registerAgentWorkingProjection folds turn_started/turn_stopped/forget into
	// it and re-broadcasts the affected workspace. Guarded by mu.
	agentWorking map[string]map[string]struct{}

	// drainWG tracks every post-commit reactor goroutine wireCallbacks registered
	// (the async delete reactor, and — once wired — the reconcile-on-open tasks) so
	// the app layer's ordered graceful shutdown (Task 15) can wait them out before
	// closing the DBs; drainCancel closes the shared drain gate to stop reactors
	// starting new work, derived from drainCtx (decisions 9 + 11). They are created
	// and stored by wireCallbacks and reached from app.Container via Drain().
	drainWG     *sync.WaitGroup
	drainCtx    context.Context
	drainCancel context.CancelFunc
}

// ReactorDrain is the shared shutdown handle for every post-commit reactor
// wireCallbacks registered. The app layer's ordered graceful shutdown (Task 15)
// closes the gate (Cancel) so reactors stop starting new work, then waits on WG
// (bounded by the shutdown deadline) before the adapter closes the DBs. Ctx is the
// cancelable parent the gate is derived from (decisions 9 + 11).
type ReactorDrain struct {
	Ctx    context.Context
	WG     *sync.WaitGroup
	Cancel context.CancelFunc
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. The workspace aggregate is backed by the singleton axWorkspace (one
// instance per type, routing every id by shard hash) built by the app layer; its
// read model lives in state/store/workspace.db and its id↔path index in view.db.
// The reviewthread aggregate owns its central per-type read model at
// state/store/review_thread.db. The agentchat aggregate owns its central
// per-type read model at state/store/agent_chat.db. terminateSession is the
// terminal-engine seam (owned by the app layer, which constructs it before
// calling New — the terminal engine has no dependency on the repositories it
// feeds) the workspace-delete cascade uses to kill a chat's live PTY before
// Forgetting it (Task 12); nil is safe (PTY teardown is skipped). The sibling
// on-disk reap seam, Container.ReapChatFiles, is NOT a parameter here — its
// resolver depends on repos.Workspace, which this very call produces, so the
// app layer assigns it on the returned *Container once the rest of the app
// layer (usecases.New) has built it; see the field's doc comment.
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	axAgentChat asynx.Asynx[domain.AgentChat],
	git wsusecase.MergeConflictChecker,
	terminateSession func(ctx context.Context, sessionID string) error,
) (*Container, error) {
	c := &Container{
		hub: h, git: git, inflight: map[string]int{},
		agentWorking: map[string]map[string]struct{}{},
		axWorkspace:  axWorkspace, axReviewThread: axReviewThread, axAgentChat: axAgentChat,
		terminateSession: terminateSession,
	}
	pathsStore, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("repositories: workspace paths: %w", err)
	}
	ws, err := workspace.New(axWorkspace, adapters.WorkspaceES(), adapters.WorkspaceView(), pathsStore)
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	// Register the hub (WS fan-out) projection on the singleton axWorkspace,
	// injecting the container-owned enrichment: every event-driven frame goes
	// through the SAME enrichFrame + hub.BroadcastWorkspace as the BeginWork/EndWork
	// rebroadcasts, so the FE spinner + merge badges survive the store/hub split
	// with zero regression (spec §3.5 hub-frame enrichment). The save-only store
	// projection is registered inside workspace.New; the two derive independently
	// from evt.Aggregate and cannot drift (decision 5).
	if err := workspace.RegisterHubProjection(axWorkspace, c.enrichFrame, c.hub.BroadcastWorkspace); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	// reviewthread owns its own central per-type read model at
	// state/store/review_thread.db (Task 12), no longer the shared view.db: pass
	// ReviewThreadView() as the read-model DB while keeping ReviewThreadES() for the
	// lazy AggregateLister Replay (§3.7).
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), adapters.ReviewThreadView(), func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.ReviewThread = rt

	// agentchat: build the asynx-backed EventStore over the singleton
	// axAgentChat, registering its store + hub projections (store.New, invoked by
	// NewEventSourced) exactly once. h.BroadcastAgentChat is the SOLE source of
	// agent-chat lifecycle frames: every agentchat.* event the agent usecase's
	// commands emit is fanned out here by the hub projection. The usecase no
	// longer broadcasts manually (that double-broadcast was retired at cutover),
	// so this projection is the one and only WS feed for agent chats.
	agentChat, err := agentchat.NewEventSourced(axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(), h.BroadcastAgentChat)
	if err != nil {
		return nil, fmt.Errorf("repositories: agent chat event store: %w", err)
	}
	c.AgentChat = agentChat
	if err := c.registerAgentWorkingProjection(); err != nil {
		return nil, fmt.Errorf("repositories: agent working projection: %w", err)
	}

	// Wire the post-commit cross-aggregate reactions (spec §3.6): the workspace
	// delete reactor + its review-thread AND agent-chat forget cascades, all
	// joined to the shared drain WaitGroup so graceful shutdown can quiesce them
	// (Task 15). Every cascaded repo must already be built — forgetDependents
	// calls into c.ReviewThread and c.AgentChat.
	if err := c.wireCallbacks(ctx, adapters.CrowbarHome()); err != nil {
		return nil, fmt.Errorf("repositories: wire callbacks: %w", err)
	}
	return c, nil
}

// WaitQuiescent blocks until every per-type asynx instance has drained its
// dispatch queue and run all projection handlers (WaitPublish = dispatcher
// WaitIdle + bus WaitForHandlers). It is the deterministic read-your-writes
// barrier: production uses the async Send path, so the store/list read model and
// the hub broadcast are INDEPENDENT projections that settle out of band; a test
// calls WaitQuiescent after a mutation so a subsequent read of ANY projection is
// guaranteed consistent — with no polling and no timeouts.
func (c *Container) WaitQuiescent() {
	c.axWorkspace.WaitPublish()
	c.axReviewThread.WaitPublish()
	c.axAgentChat.WaitPublish()
}

// wireCallbacks registers the app-level cross-aggregate reactions on the singleton
// asynx instances (spec §3.6), mirroring quiver's container wireCallbacks. It
// creates and stores the shared drain WaitGroup + cancelable drain context every
// reactor it registers joins (decisions 9 + 11), reachable by the app layer via
// Drain() for the ordered graceful shutdown (Task 15). Today it registers the
// workspace delete reactor (Task 8) with the composed forgetDependents cascade
// (review threads + agent chats, Task 12) and the bounded fs worktree delete; the
// two hub projections are already registered at construction (workspace hub in
// New above via RegisterHubProjection; reviewthread hub inside reviewthread.New),
// so re-registering them here would double-subscribe and double-broadcast — they
// are deliberately left where the live wiring puts them.
func (c *Container) wireCallbacks(
	ctx context.Context,
	crowbarHome string,
) error {
	c.drainWG = &sync.WaitGroup{}
	//nolint:gosec // G118: drainCancel is deliberately retained on the container and invoked later by the app layer's graceful shutdown via Drain().Cancel, not leaked.
	c.drainCtx, c.drainCancel = context.WithCancel(ctx)

	// The reactor lives under workspace/internal (unimportable from this out-of-tree
	// container), so it is registered through the repository's own seam, which hands
	// it the singleton axWorkspace + read model + id↔path map it holds privately.
	// The reactor's signature is a single func(ctx, wsID) error, so a new dependent
	// aggregate's cascade (Task 12's agent chats) is added by composing it into
	// forgetDependents rather than by widening the reactor itself.
	registrar, ok := c.Workspace.(workspace.DeleteReactorRegistrar)
	if !ok {
		return fmt.Errorf("workspace repository does not support delete-reactor registration")
	}
	if err := registrar.RegisterDeleteReactor(c.forgetDependents, worktreeRemover(crowbarHome), c.drainWG); err != nil {
		return fmt.Errorf("delete reactor: %w", err)
	}
	return nil
}

// Drain exposes the shared reactor drain handle so the app layer's ordered graceful
// shutdown (Task 15) can quiesce every reactor wireCallbacks registered: Cancel
// closes the gate, then it waits on WG (bounded by the shutdown deadline) before the
// adapter closes the DBs (decisions 9 + 11).
func (c *Container) Drain() ReactorDrain {
	return ReactorDrain{Ctx: c.drainCtx, WG: c.drainWG, Cancel: c.drainCancel}
}

// forgetReviewThreads is the review-thread half of the workspace delete cascade
// (spec §3.6): every review thread anchored to the deleted workspace is Forgotten,
// and each Forget's synchronous OnForget drops that thread's read-model row. It is
// injected into the async delete reactor by wireCallbacks and runs post-commit, off
// the synchronous write path.
func (c *Container) forgetReviewThreads(
	ctx context.Context,
	wsID string,
) error {
	threads, err := c.ReviewThread.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list review threads for %q: %w", wsID, err)
	}
	for _, t := range threads {
		if err := c.ReviewThread.DeleteThread(ctx, t.ID); err != nil {
			return fmt.Errorf("repositories: delete cascade: forget review thread %q: %w", t.ID, err)
		}
	}
	return nil
}

// forgetDependents runs every workspace-scoped forget cascade for a deleted
// workspace, in sequence: review threads, then agent chats (and their live
// PTYs, Task 12). It is the single callback wireCallbacks hands the async
// delete reactor (spec §3.6) — the reactor's signature is one
// func(ctx, wsID) error, so a newly added dependent aggregate's cascade is
// wired in here, at the composition point, without ever touching the reactor
// itself. Either half failing aborts the whole cascade (leaving the tombstone
// for a re-drive) rather than silently proceeding to rm -rf the worktree with
// a dependent aggregate still un-cascaded.
func (c *Container) forgetDependents(
	ctx context.Context,
	wsID string,
) error {
	if err := c.forgetReviewThreads(ctx, wsID); err != nil {
		return err
	}
	return c.forgetAgentChats(ctx, wsID)
}

// forgetAgentChats is the agent-chat half of the workspace delete cascade
// (Task 12): every AgentChat anchored to the deleted workspace is Forgotten,
// purging it outright — the owning workspace is gone and the chat has nowhere
// left to live, mirroring forgetReviewThreads/DeleteThread. It enumerates via
// ListByWorkspace so a chat's event log + read row can never be left orphaned
// after the workspace is gone.
//
// Before forgetting a chat, terminateActiveSegment kills its active segment's
// live vendor-CLI PTY (if any) — BEST-EFFORT: a terminate failure is logged
// and the Forget proceeds anyway. An orphaned PTY is a far smaller harm than
// wedging the whole workspace delete (worktree never reaped, chats never
// Forgotten) on a terminate error the cascade has no way to re-drive.
//
// After forgetting a chat, reapAgentChatFiles removes that chat's own on-disk
// directory via the injected ReapChatFiles seam — also best-effort, for the
// same reason: an orphaned plaintext ledger under a shared chats dir is far
// smaller harm than wedging the cascade on a filesystem error.
func (c *Container) forgetAgentChats(
	ctx context.Context,
	wsID string,
) error {
	chats, err := c.AgentChat.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list agent chats for %q: %w", wsID, err)
	}
	for _, chat := range chats {
		// Unbind the chat's segments from the in-memory registry BEFORE the PTY
		// teardown, so the teardown's async reconcile no-ops at ChatFor rather than
		// racing onForget's row-delete and resurrecting the chat (same fix as the
		// standalone PurgeChat path).
		if c.ForgetChatRegistry != nil {
			c.ForgetChatRegistry(chat.ID)
		}
		c.terminateActiveSegment(ctx, chat)
		if err := c.AgentChat.Forget(ctx, chat.ID); err != nil {
			return fmt.Errorf("repositories: delete cascade: forget agent chat %q: %w", chat.ID, err)
		}
		c.reapAgentChatFiles(ctx, wsID, chat.ID)
	}
	return nil
}

// reapAgentChatFiles best-effort removes a single forgotten chat's on-disk
// directory via the injected ReapChatFiles seam. A nil seam (no app layer
// wired — most of this package's own tests) is a no-op; a reap failure is
// LOGGED, never returned: it must not abort forgetAgentChats' loop over the
// remaining chats, matching terminateActiveSegment's best-effort contract.
func (c *Container) reapAgentChatFiles(
	ctx context.Context,
	wsID string,
	chatID string,
) {
	if c.ReapChatFiles == nil {
		return
	}
	if err := c.ReapChatFiles(ctx, wsID, chatID); err != nil {
		slog.ErrorContext(ctx, "repositories: delete cascade: reap agent chat files (best-effort, continuing)",
			"workspace_id", wsID, "chat_id", chatID, "err", err)
	}
}

// terminateActiveSegment best-effort terminates chat's active segment's live
// vendor-CLI PTY via the injected terminal-engine seam (c.terminateSession,
// Task 12). A chat with no active segment (every segment already ended), an
// active segment with no terminal session recorded, or a nil terminateSession
// (a test that doesn't exercise PTY teardown) are all no-ops. A terminate
// failure is LOGGED, never returned: it must not block the caller's Forget
// (see forgetAgentChats). ErrSessionNotFound (the CLI already exited) is
// already swallowed by the injected seam (app.terminateAgentSession).
func (c *Container) terminateActiveSegment(
	ctx context.Context,
	chat domain.AgentChat,
) {
	if c.terminateSession == nil || chat.ActiveSegmentID == "" {
		return
	}
	for _, seg := range chat.Segments {
		if seg.ID != chat.ActiveSegmentID || seg.TerminalSessionID == "" {
			continue
		}
		if err := c.terminateSession(ctx, seg.TerminalSessionID); err != nil {
			slog.ErrorContext(ctx, "repositories: delete cascade: terminate agent chat PTY (best-effort, continuing)",
				"chat_id", chat.ID, "terminal_session_id", seg.TerminalSessionID, "err", err)
		}
		return
	}
}

// enrichFrame builds the WS frame for ws: it attaches the two derived overlays
// that are NOT part of the event-sourced aggregate — the Working/inflight spinner
// (bracketed by BeginWork/EndWork, 00 §4) and the merge-eligibility overlay
// (CanMergeLocally/ParentBranch, resolved off the hot path from the row's
// repo-scoped siblings, spec §10) — and returns the wire DTO. It is the SINGLE
// enrichment both the hub projection (RegisterHubProjection) and the
// BeginWork/EndWork rebroadcasts converge on, so the emitted frame is identical
// regardless of trigger (spec §3.5 hub-frame enrichment).
func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	ws.Working = c.IsWorking(ws.ID) || c.agentWorkingFor(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	return dto.WorkspaceDTOFrom(ws, elig)
}

// broadcastWorkspace enriches ws and pushes it to the hub. It backs the
// BeginWork/EndWork rebroadcasts (which fire on the 202 ack, not on an event) and
// routes through the SAME enrichFrame as the hub projection so both agree.
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	c.hub.BroadcastWorkspace(c.enrichFrame(ctx, ws))
}

// BeginWork marks the start of a background mutation on the workspace and
// immediately re-broadcasts its row with Working=true, so the client spinner
// starts the moment the 202 is written — not when the op's first event lands.
// Blank ids (a create that has not produced an entity yet) are ignored.
// Concurrent ops on the same workspace nest: the overlay stays true until the
// matching EndWork of the LAST one.
func (c *Container) BeginWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	c.inflight[wsID]++
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// EndWork marks the end of a background mutation on the workspace and
// re-broadcasts its row so the final frame always carries Working=false (and
// whatever LastError the op recorded). Unbalanced calls never underflow, and a
// row deleted by the op itself just skips the re-broadcast (its tombstone
// already rode the event stream).
func (c *Container) EndWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	if n := c.inflight[wsID]; n <= 1 {
		delete(c.inflight, wsID)
	} else {
		c.inflight[wsID] = n - 1
	}
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// IsWorking reports whether the workspace has a background mutation in flight.
func (c *Container) IsWorking(
	wsID string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight[wsID] > 0
}

// rebroadcast pushes the workspace's current row to the hub so an overlay
// transition (Begin/EndWork) is visible without waiting for the op's next
// event. Best-effort: a missing row (already deleted) skips silently.
func (c *Container) rebroadcast(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

// registerAgentWorkingProjection subscribes a THIRD projection on axAgentChat
// (alongside the store + hub projections built in NewEventSourced): it re-derives
// the per-workspace Working overlay from agent turn events (00 §7.4). turn_started
// marks the chat working, turn_stopped clears it, and a Forget of a chat mid-turn
// clears it too (so a delete never wedges the spinner on); each transition
// re-broadcasts the affected workspace through the same enrichFrame path the
// inflight overlay uses, so the FE spinner on the workspace tree + context pill +
// tiles tracks live agent activity. The in-memory set is authoritative (not a
// read-model query), so it never races the store projection.
//
// Only turn_started/turn_stopped are folded — a mid-turn segment_ended (process
// exit, switch-out, boot reconcile) is ALWAYS accompanied by a StopTurn
// (agent.Usecase.endSegmentAndMaybeStopTurn), so a turn_stopped always arrives to
// clear the set; segment_ended needs no separate handling.
//
// Boot note (in-memory overlay, empty on restart): agentWorking starts EMPTY on
// daemon boot, so a chat that was mid-turn when the daemon stopped shows idle
// until its next turn_started. That is safe because boot-reconcile
// (agent.Usecase.ReconcileOnBoot) ends stale segments/turns on restart, so no
// chat is genuinely mid-turn-but-shown-idle after boot — consistent with the FE
// chat-row working map's accepted default-idle-on-load.
func (c *Container) registerAgentWorkingProjection() error {
	if _, err := c.axAgentChat.Subscribe(asynx.Topic("agentchat.*"),
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			switch agentEventKind(evt.EventName) {
			case "turn_started":
				c.setAgentTurn(wsID, evt.AggregateID, true)
			case "turn_stopped":
				c.setAgentTurn(wsID, evt.AggregateID, false)
			default:
				return
			}
			c.rebroadcast(ctx, wsID)
		}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	if _, err := c.axAgentChat.OnForget(
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			c.setAgentTurn(wsID, evt.AggregateID, false)
			c.rebroadcast(ctx, wsID)
		}); err != nil {
		return fmt.Errorf("onforget: %w", err)
	}
	return nil
}

// setAgentTurn adds/removes chatID from the workspace's mid-turn set under mu.
func (c *Container) setAgentTurn(wsID, chatID string, working bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if working {
		set := c.agentWorking[wsID]
		if set == nil {
			set = map[string]struct{}{}
			c.agentWorking[wsID] = set
		}
		set[chatID] = struct{}{}
		return
	}
	if set := c.agentWorking[wsID]; set != nil {
		delete(set, chatID)
		if len(set) == 0 {
			delete(c.agentWorking, wsID)
		}
	}
}

// agentWorkingFor reports whether the workspace has any agent chat mid-turn.
func (c *Container) agentWorkingFor(wsID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.agentWorking[wsID]) > 0
}

// agentEventKind extracts <kind> from an agentchat EventName ("agentchat.<kind>.<id>").
func agentEventKind(eventName string) string {
	rest := strings.TrimPrefix(eventName, "agentchat.")
	kind, _, found := strings.Cut(rest, ".")
	if !found {
		return rest
	}
	return kind
}

// eligibilityFor resolves the merge-eligibility overlay (incl. the predicted
// merge-conflict flag) for ws by reading its siblings and delegating to the
// shared wsusecase.ResolveMergeEligibility — the SAME resolver the snapshot read
// path uses, so the live broadcast and the snapshot always agree. The sibling
// read is best effort — a failed List degrades to no eligibility rather than
// dropping the broadcast.
func (c *Container) eligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
) wsusecase.MergeEligibility {
	if ws.ParentID == "" {
		return wsusecase.MergeEligibility{}
	}
	siblings, err := c.ListWorkspacesInRepo(ctx, ws.ProjectID, ws.RepoID)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibility(ctx, ws, siblings, c.git)
}

// ListWorkspaces returns every workspace row with the derived Working overlay
// applied, so a snapshot-on-subscribe taken mid-mutation agrees with the live
// broadcast frames. It backs the Workspaces snapshot-on-subscribe.
func (c *Container) ListWorkspaces(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID) || c.agentWorkingFor(rows[i].ID)
	}
	return rows, nil
}

// worktreeRemover builds the bounded fs delete the async delete reactor uses to
// rm -rf a deleted workspace's ENTIRE on-disk footprint, off the synchronous
// write path (spec §3.5/§3.6, decision 9). Since the workspace-root split, path
// (the id↔path map's stored WorktreePath) is the "worktree" leaf of a workspace
// root that also holds the sibling "chats" tree (worktreepath.WorkspaceRoot /
// ChatsDir); `git worktree remove` only clears the "worktree" leaf itself, so
// this removes path's PARENT directory instead — nuking the git checkout and
// any agentic chat state (ledger + segment tmp dirs) together in one rm -rf.
// worktreepath.WorkspaceRoot cannot be imported here (this package sits outside
// the usecases/ tree that internal package is scoped to; Go's internal-package
// visibility forbids it), so the parent is computed inline via filepath.Dir —
// byte-for-byte the same computation. It is GUARDED by the crowbar home: only a
// CROWBAR-MANAGED worktree (a path strictly under the home) is ever removed. An
// adopted home / main worktree's id↔path entry is the user's REAL checkout
// (repo.Path / project.Path, which live OUTSIDE the home, with no "worktree"
// leaf of their own) and must NEVER be deleted — the guard mirrors the
// synchronous project-delete removeWorktreeIfCrowbarManaged so both the delete
// reactor and the boot orphan-sweep (bootSweepPurge, api/internal/app/container.go)
// converge without ever destroying a user's repository. A blank path, a path
// outside the home, or an already-gone dir is an idempotent no-op (os.RemoveAll
// returns nil for a missing path), so a crash re-driven cascade rm's to nothing.
func worktreeRemover(
	crowbarHome string,
) func(path string) error {
	return func(path string) error {
		if !managedWorktreePath(path, crowbarHome) {
			if path != "" {
				slog.Warn("repositories: refusing to rm worktree outside the crowbar home",
					"path", path, "home", crowbarHome)
			}
			return nil
		}
		// The removed target is the PARENT of the worktree leaf (the workspace
		// root holding the sibling chats tree). Re-guard the ROOT itself: a
		// degenerate one-segment leaf (<home>/worktree) has filepath.Dir == home,
		// and rm'ing that would nuke the ENTIRE crowbar home. Only a root that is
		// still STRICTLY under home — i.e. path had an intermediate segment below
		// home — is ever removed.
		root := filepath.Dir(path)
		if !managedWorktreePath(root, crowbarHome) {
			slog.Warn("repositories: refusing to rm workspace root at or above the crowbar home",
				"root", root, "path", path, "home", crowbarHome)
			return nil
		}
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("repositories: remove workspace root %q: %w", root, err)
		}
		return nil
	}
}

// managedWorktreePath reports whether path is strictly under the crowbar home: a
// non-empty path with home as a proper directory-boundary prefix. Adopted
// checkouts (repo.Path / project.Path) live outside the home and are excluded, so
// a delete/sweep never rm's the user's real repository; and because the check is
// strict (home itself is not "under" home), applying it to the removal ROOT also
// blocks the degenerate case where the root would be the home directory itself
// (spec §3.9; the locked workspace-model law).
func managedWorktreePath(
	path string,
	crowbarHome string,
) bool {
	if path == "" || crowbarHome == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(crowbarHome, "/")+"/")
}

// ListWorkspacesInRepo returns every workspace row scoped to one project+repo,
// with the derived Working overlay applied, read from MY central store read
// model (state/store/workspace.db) filtered by project_id/repo_id via the
// workspace repo's ListInRepo — a single central-store read, not the
// whole-install per-entity scan the retired workspace_directory projection was
// built to avoid. It backs the repo-scoped snapshot-on-subscribe builders and
// the merge-eligibility overlay.
func (c *Container) ListWorkspacesInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces in repo: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID) || c.agentWorkingFor(rows[i].ID)
	}
	return rows, nil
}
