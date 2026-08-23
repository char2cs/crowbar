package repositories

import (
	"context"
	"errors"
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
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	ReviewThread reviewthread.ReviewThread
	// AgentChat is the asynx-backed EventStore: its store/hub projections are
	// live on axAgentChat and the agent usecase sends every AgentChat mutation
	// through it (the gorm-backed store was retired in the Task 10 cutover).
	AgentChat agentchat.EventStore
	// AgentActivity is the conversation record: turns, tool calls, subagents and
	// interruptions, in their own aggregate with their own read model. It replaced
	// the flat-file ledger, which could represent none of those.
	AgentActivity agentactivity.EventStore
	// AgentRunner is the asynx-backed EventStore for the running vendor CLI — the
	// thing that MOVES between chats on /clear and /resume. Its store/hub
	// projections are live on axAgentRunner, and the agent usecase now sends every
	// runner command through it. The workspace-delete cascade reads it too, to find
	// the CLI pointed at a chat it is about to Forget.
	AgentRunner agentrunner.EventStore
	hub         hub.WebSocketHub
	git         wsusecase.MergeConflictChecker
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
	// removes that chat's own <chatsDir>/<chatID> directory — its handoff ledger,
	// and nothing else. No tmp dir lives under a chat any more (runner tmp dirs
	// now live at <chatsDir>/runners/<runnerID>-<provider>, worktreepath.RunnerDir,
	// reaped by the runner's own lifecycle, not this cascade), so this closes the
	// gap where Forgetting the event-sourced aggregate left its PLAINTEXT on-disk
	// footprint behind under .crowbar. Unlike terminateSession (a New(...)
	// constructor parameter), this is a settable field the app layer assigns
	// AFTER construction: the real implementation (app.reapAgentChatFiles) reuses
	// the SAME agent.WorkspaceReader
	// (AgentChatsDir + WorktreeDir) and agent.RemoveUnderHome guard the standalone
	// PurgeChat path already routes through, and that reader is built from
	// repos.Workspace — which does not exist until repositories.New returns — so
	// it cannot be threaded in as a constructor argument without a genuine
	// construction cycle (usecases.New itself takes *repositories.Container).
	// Exported (not routed through a setter) so tests can inject a fake exactly
	// like they already do for c.Workspace. Nil is safe: reaping is skipped.
	ReapChatFiles func(ctx context.Context, wsID, chatID string) error
	// axWorkspace/axReviewThread/axAgentChat/axAgentRunner are the per-type asynx
	// instances, retained so WaitQuiescent can drain their dispatch queues +
	// projection handlers — the deterministic read-your-writes barrier for tests (no
	// polling, no timeouts).
	axWorkspace     asynx.Asynx[domain.Workspace]
	axReviewThread  asynx.Asynx[domain.ReviewThread]
	axAgentChat     asynx.Asynx[domain.AgentChat]
	axAgentActivity asynx.Asynx[domain.AgentActivity]
	axAgentRunner   asynx.Asynx[domain.AgentRunner]
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
	drainGate   *drain.Gate
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
	Gate   *drain.Gate
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
	axAgentActivity asynx.Asynx[domain.AgentActivity],
	axAgentRunner asynx.Asynx[domain.AgentRunner],
	git wsusecase.MergeConflictChecker,
	terminateSession func(ctx context.Context, sessionID string) error,
) (*Container, error) {
	c := &Container{
		hub: h, git: git, inflight: map[string]int{},
		agentWorking: map[string]map[string]struct{}{},
		axWorkspace:  axWorkspace, axReviewThread: axReviewThread, axAgentChat: axAgentChat,
		axAgentRunner:    axAgentRunner,
		axAgentActivity:  axAgentActivity,
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
	agentChat, err := agentchat.NewEventSourced(axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(),
		// Bridged inline until the fanout lands (plan task 4), which takes this
		// decision out of the repository layer entirely.
		func(e agentchat.ChatEvent) {
			h.BroadcastAgentChat(e.ChatID, e.WorkspaceID, e.Kind, e.Working && !e.Forgotten)
		})
	if err != nil {
		return nil, fmt.Errorf("repositories: agent chat event store: %w", err)
	}
	c.AgentChat = agentChat
	if err := c.registerAgentWorkingProjection(); err != nil {
		return nil, fmt.Errorf("repositories: agent working projection: %w", err)
	}

	// agentactivity: the conversation record, over its OWN event log, snapshot
	// store and read model. Its content store lives beside the state directory so
	// tool payloads are swept by the same retention policy as the rest of it.
	agentActivity, err := agentactivity.NewEventSourced(
		axAgentActivity, adapters.AgentActivityES(), adapters.AgentActivityReadDB(),
		filepath.Join(adapters.CrowbarHome(), "state", "content"))
	if err != nil {
		return nil, fmt.Errorf("repositories: agent activity event store: %w", err)
	}
	c.AgentActivity = agentActivity

	// agentrunner: build the asynx-backed EventStore over the singleton
	// axAgentRunner, registering its two read projections (live runners +
	// append-only conversation history) and its hub projection exactly once, over
	// its OWN per-type planes (state/events/agent_runner.db and
	// state/store/agent_runner.db). h.BroadcastAgentRunner is the sole source of
	// runner lifecycle frames (started/session_bound/moved/displaced/exited). The agent
	// usecase sends every runner command through this store, and the workspace-delete
	// cascade below reads it to find the CLI pointed at a chat it is about to Forget.
	agentRunner, err := agentrunner.NewEventSourced(
		axAgentRunner, adapters.AgentRunnerES(), adapters.AgentRunnerReadDB(), h.BroadcastAgentRunner)
	if err != nil {
		return nil, fmt.Errorf("repositories: agent runner event store: %w", err)
	}
	c.AgentRunner = agentRunner

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
	c.axAgentActivity.WaitPublish()
	c.axAgentRunner.WaitPublish()
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
	c.drainGate = drain.New()
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
	if err := registrar.RegisterDeleteReactor(c.forgetDependents, worktreeRemover(crowbarHome), c.drainGate); err != nil {
		return fmt.Errorf("delete reactor: %w", err)
	}
	return nil
}

// Drain exposes the shared reactor drain handle so the app layer's ordered graceful
// shutdown (Task 15) can quiesce every reactor wireCallbacks registered: Cancel
// closes the gate, then it waits on WG (bounded by the shutdown deadline) before the
// adapter closes the DBs (decisions 9 + 11).
func (c *Container) Drain() ReactorDrain {
	return ReactorDrain{Ctx: c.drainCtx, Gate: c.drainGate, Cancel: c.drainCancel}
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

// ForgetWorkspaceDependents runs the workspace-scoped forget cascade (review threads,
// then agent chats and their conversations) for a workspace that is going away. It is the
// SAME cascade the async delete reactor runs, exposed so the boot orphan-sweep can re-drive
// it: a delete whose reactor never ran to completion — it crashed mid-cascade, or the
// drain gate refused it during shutdown — leaves the workspace tombstoned with its chat
// aggregates still un-forgotten, and the boot sweep is the only thing that re-drives such a
// tombstone. Without this the sweep rm'd the worktree and Forgot the WORKSPACE only, and
// GetChat kept resolving a chat pointing at a workspace that no longer exists.
func (c *Container) ForgetWorkspaceDependents(
	ctx context.Context,
	wsID string,
) error {
	return c.forgetDependents(ctx, wsID)
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

// forgetAgentChats is the agent-chat half of the workspace delete cascade: every
// AgentChat anchored to the deleted workspace is Forgotten, purging it outright —
// the owning workspace is gone and the chat has nowhere left to live, mirroring
// forgetReviewThreads/DeleteThread. It enumerates via ListByWorkspace so a chat's
// event log + read row can never be left orphaned after the workspace is gone.
//
// It is the cascade twin of agent.ChatUsecase.PurgeChat and follows the same ORDER,
// for the same reason: Forget the chat FIRST, then kill the CLI pointed at it.
// The PTY teardown fires the runner-exit reconcile asynchronously, and that path
// writes to the chat (it closes a turn the dead CLI left open); a chat command
// that commits BEFORE the Forget can have its read-model Save land AFTER Forget's
// row-delete and resurrect the chat as a zombie row. Forgetting first erases the
// event log, so every later chat command fails Validate and emits nothing at all
// — the zombie becomes unrepresentable rather than merely unlikely.
//
// Everything after the Forget is BEST-EFFORT (logged, never returned): an orphaned
// PTY, a leftover conversation row or a leftover ledger dir are all far smaller
// harms than wedging the whole workspace delete — worktree never reaped, remaining
// chats never Forgotten — on an error the cascade has no way to re-drive.
func (c *Container) forgetAgentChats(
	ctx context.Context,
	wsID string,
) error {
	chats, err := c.AgentChat.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list agent chats for %q: %w", wsID, err)
	}
	for _, chat := range chats {
		if err := c.AgentChat.Forget(ctx, chat.ID); err != nil {
			return fmt.Errorf("repositories: delete cascade: forget agent chat %q: %w", chat.ID, err)
		}
		c.retireChatRunners(ctx, chat.ID)
		if err := c.AgentRunner.ForgetChat(ctx, chat.ID); err != nil {
			slog.ErrorContext(ctx, "repositories: delete cascade: forget chat conversations (best-effort, continuing)",
				"workspace_id", wsID, "chat_id", chat.ID, "err", err)
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

// retireChatRunners best-effort takes EVERY vendor CLI on chatID off that chat and kills it
// — the cascade twin of the agent runner concern's retireChatRunners, in the same order and for the same
// reasons.
//
// The PLURAL read, not the single-row one: this is a delete, and a delete is precisely where
// you want everyone gone. If the invariant were transiently broken (two placements racing
// this chat), killing "the" runner would leave the other alive and running against a chat
// that is about to be Forgotten.
//
// Displace FIRST: the chat is being erased, and a SIGTERM does not kill a process
// synchronously, so until the CLI falls over it would otherwise still be pointed at a chat
// that no longer exists — where its hooks would try to write, and where a read could still
// hand it out. Displacement is a PLACEMENT fact (ours alone) and says nothing about
// liveness, so it is safe to record even though the process is still running.
//
// Kill SECOND, and never hand-delete the runner's row: the PTY is the sole authority on
// liveness, so the row goes when the process does (onExit → Exit → the projection drops it).
// Reaching into the read model to delete it would make this package a second authority on
// liveness — the exact drift this model deletes.
//
// A dormant chat and a nil terminateSession (a test that doesn't exercise PTY teardown) are
// both no-ops. Failures are LOGGED, never returned: they must not block the cascade (see
// forgetAgentChats). ErrSessionNotFound (the CLI already exited) is already swallowed by the
// injected seam (app.terminateAgentSession).
func (c *Container) retireChatRunners(
	ctx context.Context,
	chatID string,
) {
	if c.terminateSession == nil {
		return
	}
	placed, err := c.AgentRunner.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "repositories: delete cascade: look up chat's runners (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	for _, live := range placed {
		// An already-EXITED runner is not an error, and must not be logged as one: a CLI
		// quitting on its own moments before the cascade reached it is the ORDINARY case, and
		// its exit has already cleared every placement this would have. Displace says so with
		// ErrValidation (see agentrunner/internal/commands/displace.go), and the agent
		// usecase's own displace() treats it exactly the same way.
		if _, err := c.AgentRunner.Displace(ctx, live.ID); err != nil &&
			!errors.Is(err, asynxModels.ErrValidation) && !errors.Is(err, agentrunner.ErrNotFound) {
			slog.ErrorContext(ctx, "repositories: delete cascade: displace agent chat runner (best-effort, continuing)",
				"chat_id", chatID, "runner_id", live.ID, "err", err)
		}
		if err := c.terminateSession(ctx, live.TerminalSession); err != nil {
			slog.ErrorContext(ctx, "repositories: delete cascade: terminate agent chat PTY (best-effort, continuing)",
				"chat_id", chatID, "runner_id", live.ID, "terminal_session_id", live.TerminalSession, "err", err)
		}
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
	ws.Working = c.WorkingFor(ws.ID)
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

// WorkingFor reports whether the workspace is working via EITHER derived overlay:
// a background mutation in flight (inflight, via IsWorking) OR an agent chat
// mid-turn (agentWorking, via agentWorkingFor). It is the single combined read the
// REST list/detail handlers stamp Working from, so a REST read agrees with both
// the live broadcast frames (enrichFrame) and the snapshot-on-subscribe readers
// (ListWorkspaces/ListWorkspacesInRepo) — all four converge on this method. Each
// overlay is read under its OWN lock acquisition (never one lock held across
// both), mirroring the existing enrichFrame combination: the two are independent
// booleans with no cross-overlay invariant, so a transient interleaving can only
// observe a value that was truthful at some instant during the call — the same
// guarantee every other reader already provides.
func (c *Container) WorkingFor(
	wsID string,
) bool {
	return c.IsWorking(wsID) || c.agentWorkingFor(wsID)
}

// rebroadcast pushes the workspace's current row to the hub so an overlay
// transition (Begin/EndWork) is visible without waiting for the op's next
// event. Best-effort: a missing row (already deleted) skips silently.
//
// A read failure here is LOGGED rather than dropped in silence, because the frame
// it costs us is not interchangeable with the next one. Working is transported as
// a delta and nothing repeats it: the transition that turns the spinner OFF is
// broadcast exactly once, so a Get that fails leaves every client spinning over an
// idle workspace until some unrelated event happens to re-broadcast the row. That
// wedge is invisible from the outside — the daemon's own state is correct and the
// event log is clean — and it stayed invisible precisely because this returned
// without a word. ErrNotFound is the expected case (the op deleted its own row)
// and is not worth a line.
func (c *Container) rebroadcast(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		if !errors.Is(err, apperr.ErrNotFound) {
			slog.WarnContext(ctx, "repositories: rebroadcast: get workspace "+
				"(client overlay may be stale until the next event)",
				"workspace_id", wsID, "err", err)
		}
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

// registerAgentWorkingProjection subscribes a THIRD projection on axAgentChat
// (alongside the store + hub projections built in NewEventSourced): it re-derives
// the per-workspace Working overlay from agent live-state events (00 §7.4), and a
// Forget of a working chat clears it too (so a delete never wedges the spinner on).
// Each transition re-broadcasts the affected workspace through the same enrichFrame
// path the inflight overlay uses, so the FE spinner on the workspace tree + context
// pill + tiles tracks live agent activity. The in-memory set is authoritative (not a
// read-model query), so it never races the store projection.
//
// The overlay MIRRORS the aggregate's own Working (evt.Aggregate.Working) rather than
// re-deriving "busy" from the event kind. The fold — a turn being open OR async work
// being in flight (domain.AgentChat.Working) — then lives in exactly ONE place, the
// write side, and this cannot drift from what the chat row and REST reads report. It is
// also why turn_stopped is NOT read as "idle" here: a chat waiting on a background task
// ends its turn but stays Working, and this reads that Working straight off the event.
// The two kinds listed are the only ones that can CHANGE it; every other agentchat event
// (created, title_set, ...) leaves it alone and must not cost a rebroadcast — see
// setAgentTurn on why a needless one is expensive.
//
// The closing event always arrives. A chat's turn is opened and closed by its hooks, and
// each turn_stop restates the async-work level; the ONE case where the closing hook never
// comes — the CLI dying — is covered by the runner-exit reconcile
// (the agent runner concern's reconcileRunnerExit → closeAbandonedTurn), which issues an AbandonTurn
// that closes the turn AND zeroes the async-work level, since neither can outlive the
// process that announced them.
//
// Boot note (in-memory overlay, empty on restart): agentWorking starts EMPTY on daemon
// boot, so a chat that was mid-turn when the daemon stopped shows idle until its next
// turn_started. Showing idle is the TRUTHFUL answer — every agent PTY dies with the
// daemon, so nothing is running — but note the chat AGGREGATE can still read Working=true
// until something closes that turn, so this overlay and a REST read of the chat can
// disagree until the runner boot reconcile lands (the next task in this series). It is
// consistent with the FE chat-row working map's accepted default-idle-on-load.
func (c *Container) registerAgentWorkingProjection() error {
	if _, err := c.axAgentChat.Subscribe(asynx.Topic("agentchat.*"),
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			var flipped bool
			switch agentEventKind(evt.EventName) {
			case "turn_started", "turn_stopped":
				flipped = c.setAgentTurn(wsID, evt.AggregateID, evt.Aggregate.Working)
			default:
				return
			}
			if flipped {
				c.rebroadcast(ctx, wsID)
			}
		}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	if _, err := c.axAgentChat.OnForget(
		func(ctx context.Context, evt asynxModels.Event[domain.AgentChat]) {
			wsID := evt.Aggregate.WorkspaceID
			if wsID == "" {
				return
			}
			if c.setAgentTurn(wsID, evt.AggregateID, false) {
				c.rebroadcast(ctx, wsID)
			}
		}); err != nil {
		return fmt.Errorf("onforget: %w", err)
	}
	return nil
}

// setAgentTurn adds/removes chatID from the workspace's mid-turn set under mu and
// reports whether the set's EMPTINESS flipped (empty↔non-empty) — i.e. whether the
// workspace's Working overlay actually changed value.
//
// Only that transition may trigger a rebroadcast. Working is a single bool derived
// from `len(set) > 0`, so a second chat starting a turn in a workspace that is
// already working, or the first of two concurrent chats stopping, changes NOTHING
// observable — yet rebroadcasting anyway is far from free: broadcastWorkspace →
// enrichFrame → eligibilityFor runs ListWorkspacesInRepo AND git.WouldMergeConflict,
// a real `git merge-tree --write-tree` subprocess taken under the per-clone git
// mutex. Firing that on every turn_started/turn_stopped made N concurrently-working
// chats in one workspace cost 2N git subprocesses per round on the shared lock —
// the exact contention shape behind this repo's history of git-mutex hangs.
//
// The transition is computed while holding mu; the caller rebroadcasts only AFTER
// this returns, so the (potentially slow, git-touching) broadcast still never runs
// under the lock.
func (c *Container) setAgentTurn(wsID, chatID string, working bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	before := len(c.agentWorking[wsID]) > 0
	if working {
		c.addAgentTurn(wsID, chatID)
	} else {
		c.clearAgentTurn(wsID, chatID)
	}
	return before != (len(c.agentWorking[wsID]) > 0)
}

// addAgentTurn marks chatID mid-turn in wsID's set. Callers hold mu.
func (c *Container) addAgentTurn(wsID, chatID string) {
	set := c.agentWorking[wsID]
	if set == nil {
		set = map[string]struct{}{}
		c.agentWorking[wsID] = set
	}
	set[chatID] = struct{}{}
}

// clearAgentTurn drops chatID from wsID's set, pruning the set once it empties so
// the map cannot grow without bound across a long-lived daemon. Callers hold mu.
func (c *Container) clearAgentTurn(wsID, chatID string) {
	set := c.agentWorking[wsID]
	if set == nil {
		return
	}
	delete(set, chatID)
	if len(set) == 0 {
		delete(c.agentWorking, wsID)
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
		rows[i].Working = c.WorkingFor(rows[i].ID)
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
// the chats tree together in one rm -rf. The chats tree is not targeted by
// name here; it survives only by accident of being the worktree's sibling
// under the same parent, which the root rm -rf takes whole.
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
		// The parent is only the right thing to delete when the path really is an
		// identity-keyed worktree. A workspace still recorded at its pre-leaf path
		// (<slug>/<branch>, the shape used before the worktree leaf existed) has
		// the SLUG directory as its parent — the directory holding every branch of
		// that repo — so removing one such workspace would take all of them.
		//
		// "Under the home" cannot catch that: the slug directory is under the home.
		// The shape is what distinguishes them, and there is exactly one shape a
		// managed worktree can have.
		if !isWorkspaceWorktree(path) {
			slog.Warn("repositories: refusing to rm a path that is not a workspace worktree",
				"path", path, "root", root)
			return nil
		}
		if err := removeWorkspaceRoot(root); err != nil {
			return fmt.Errorf("repositories: remove workspace root %q: %w", root, err)
		}
		// The root IS the workspace's whole on-disk footprint — worktree, chats
		// and storages — so the rm above took everything the workspace owned. The
		// navigable alias that pointed into it is withdrawn by the delete usecase,
		// which knows the slug and branch that name it; anything that outlives a
		// crash is cleared by SweepDanglingAliases at boot.
		pruneEmptiedWorkspaceParents(root, crowbarHome)
		return nil
	}
}

// SweepDanglingAliases removes every navigable alias under crowbarHome whose
// target no longer exists, and the directories that leaves empty.
//
// The delete path withdraws a workspace's alias itself, where the slug and
// branch that name it are known. This is the crash net: a daemon killed between
// removing a root and withdrawing its alias leaves a broken link in the tree a
// human browses. Running it ONCE at boot costs one walk of the projects tree
// instead of one per delete.
//
// A dangling link is the only signature it acts on, so a live alias — one
// pointing at a root that still exists — is never a candidate.
func SweepDanglingAliases(crowbarHome string) int {
	if crowbarHome == "" {
		return 0
	}
	projects := filepath.Join(crowbarHome, "projects")
	var emptied []string
	removed := 0
	_ = filepath.WalkDir(projects, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink == 0 {
			return nil //nolint:nilerr // an unreadable branch of the tree is skipped, never fatal
		}
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		if rmErr := os.Remove(path); rmErr != nil {
			slog.Warn("repositories: unlink dangling alias", "path", path, "err", rmErr)
			return nil
		}
		removed++
		emptied = append(emptied, filepath.Dir(path))
		return nil
	})
	for _, dir := range emptied {
		for d := dir; managedWorktreePath(d, projects); d = filepath.Dir(d) {
			if os.Remove(d) != nil {
				break
			}
		}
	}
	if removed > 0 {
		slog.Info("repositories: cleared dangling workspace aliases", "count", removed)
	}
	return removed
}

// workspaceRootOwned are the entries a workspace root may lose. The first four
// are the directories Crowbar itself creates; .DS_Store is macOS metadata that
// appears the moment the user opens the root in Finder, and keeping the root
// alive for it would turn every browsed workspace into permanent litter.
var workspaceRootOwned = []string{"worktree", "chats", "storages", "threads", ".DS_Store"}

// removeWorkspaceRoot deletes the workspace's own directories, then the root —
// which succeeds only once nothing else is left in it.
//
// It replaces an rm -rf of the entire root. That was written to the rule "the
// root IS the workspace's whole on-disk footprint", and on a real machine it is
// not: a <slug>/<branch> root was found holding five hand-made git worktrees
// beside the managed one, so deleting that workspace would have taken 4.5GB of
// checkouts Crowbar never created. A foreign entry now keeps the root alive and
// is reported instead of destroyed.
func removeWorkspaceRoot(
	root string,
) error {
	for _, name := range workspaceRootOwned {
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	err := os.Remove(root)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	rest, readErr := os.ReadDir(root)
	if readErr != nil {
		return err
	}
	kept := make([]string, 0, len(rest))
	for _, e := range rest {
		kept = append(kept, e.Name())
	}
	slog.Warn("repositories: workspace root kept; it holds entries crowbar did not create",
		"root", root, "kept", kept)
	return nil
}

// pruneEmptiedWorkspaceParents removes the directories a workspace-root removal
// emptied, walking up from the root's parent.
//
// It cannot delete anything it did not empty and it cannot climb out of the
// project. os.Remove only ever succeeds on an EMPTY directory, so a slug still
// holding a sibling workspace stops the walk on its own; and the floor is
// <home>/projects/<projectID>, which is never a candidate — that level holds the
// project's icon, its `workspaces` state and its repo directories beside the
// slug trees, so climbing into it would delete live state rather than litter.
func pruneEmptiedWorkspaceParents(
	root string,
	crowbarHome string,
) {
	floor, ok := projectDirOf(root, crowbarHome)
	if !ok {
		return
	}
	for dir := filepath.Dir(root); managedWorktreePath(dir, floor); dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			return
		}
	}
}

// projectDirOf returns <home>/projects/<projectID> for a path beneath it, and
// false for anything not laid out that way — an adopted checkout, or a path the
// managed layout does not explain, neither of which may be climbed.
func projectDirOf(
	path string,
	crowbarHome string,
) (string, bool) {
	rel, err := filepath.Rel(crowbarHome, path)
	if err != nil {
		return "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "projects" || parts[1] == "" || parts[1] == ".." {
		return "", false
	}
	return filepath.Join(crowbarHome, parts[0], parts[1]), true
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

// isWorkspaceWorktree reports whether a path is a worktree whose PARENT is a
// workspace root — the one thing that makes deleting that parent safe.
//
// The test is the "worktree" leaf, and it holds for both managed layouts: the
// identity-keyed <...>/workspaces/<id>/worktree and the older name-keyed
// <slug>/<branch>/worktree both put the worktree inside its own workspace's
// root. What it excludes is the PRE-LEAF shape, <slug>/<branch>, whose parent is
// the slug directory holding every branch of the repo.
//
// A shape test, not a location test — managedWorktreePath already answers "is
// this ours".
func isWorkspaceWorktree(path string) bool {
	return filepath.Base(path) == "worktree"
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
		rows[i].Working = c.WorkingFor(rows[i].ID)
	}
	return rows, nil
}
