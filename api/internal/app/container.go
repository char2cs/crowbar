package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/realtime"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

// Container is the application layer: the hub, the aggregate repositories, the
// GORM CRUD stores, the composed usecases, and the realtime service owning the
// lazy file-watcher and LSP lifecycles.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
	Usecases     *usecases.Container
	Realtime     *realtime.Service

	// engines is retained for ONE reason: Shutdown must quiesce the terminal engine
	// (kill every PTY and JOIN the exit callbacks those deaths fire) BEFORE it drains
	// the aggregates below, because those callbacks are writers into them. See
	// Shutdown step 1.
	engines *engine.Container

	// axWorkspace, axReviewThread, axAgentChat, and axAgentRunner are the per-type
	// asynx singletons (one per aggregate type, routing every id by shard hash).
	// They are retained here so Task 15's ordered graceful shutdown can drain each
	// via ax.Shutdown. axAgentRunner carries the RUNNER aggregate — one live vendor CLI
	// in one PTY, the thing that moves between chats on /clear and /resume.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.Chat]
	// axAgentActivity is the conversation record's own per-type singleton. It is
	// separate from axAgentChat because their write rates differ by orders of
	// magnitude: a chat emits a handful of events, its activity emits hundreds per
	// turn, and sharing one single-writer event log would put a sidebar repaint
	// behind a tool-call storm.
	axAgentActivity asynx.Asynx[domain.ChatActivity]
	axAgentRunner   asynx.Asynx[agents.Runner]
	// axNode is the Node position aggregate's own per-type singleton
	// (2026-09-08 sidebar-placement-unification): the ONE entity that will own
	// every sidebar row's ParentID/Order. This wiring is purely additive —
	// nothing else in the codebase reads or writes it yet.
	axNode asynx.Asynx[domain.Node]
}

// New constructs the application layer from the engine and adapter containers
// and wires the aggregate repositories into the hub (00 §7).
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES(), adapters.ReviewThreadSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}
	// One eager axWorkspace singleton over the per-type event log, routing every
	// workspace id to a shard by hash (decision 1) — replaces the per-entity
	// AsynxFactory the repository used to resolve per workspace.
	// SchemaVersion/StripRetiredPlacementFields: real production workspaces
	// dragged or reordered before the sidebar-placement-unification migration
	// carry /order and /folderId patches domain.Workspace no longer has fields
	// for — see StripRetiredPlacementFields's own doc.
	axWorkspace, err := newAsynx[domain.Workspace](adapters.WorkspaceES(), adapters.WorkspaceSS(),
		func(b *asynx.Builder[domain.Workspace]) {
			b.WithSchemaVersion(workspace.SchemaVersion).
				WithUpcaster(1, workspace.StripRetiredPlacementFields)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	// axAgentChat is the per-type singleton over state/events/agent_chat.db: it
	// is built and its store/hub projections registered (via repositories.New ->
	// agentchat.NewEventSourced), and the agent usecase now sends every AgentChat
	// mutation through it (the gorm-backed store was retired in the Task 10
	// cutover).
	axAgentChat, err := newAsynx[domain.Chat](adapters.AgentChatES(), adapters.AgentChatSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent chat: %w", err)
	}
	// axAgentRunner is the per-type singleton over state/events/agent_runner.db: the
	// running vendor CLI is now a modelled aggregate of its own, so moving one
	// between chats (/clear, /resume) is a single write to a single aggregate
	// instead of a delete-here/insert-there across two chats with no transaction.
	// Built and its projections registered (via repositories.New ->
	// runner.NewEventSourced); nothing SENDS runner commands yet — that
	// cutover is a later task — so it is additive for now.
	axAgentActivity, err := newAsynx[domain.ChatActivity](
		adapters.AgentActivityES(), adapters.AgentActivitySS(),
	)
	if err != nil {
		return nil, fmt.Errorf("app container: agent activity asynx: %w", err)
	}

	axAgentRunner, err := newAsynx[agents.Runner](adapters.AgentRunnerES(), adapters.AgentRunnerSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent runner: %w", err)
	}

	axNode, err := newAxNode(adapters)
	if err != nil {
		return nil, err
	}

	gormStores, err := newGORMStores(adapters.GlobalView())
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	chatSnapshots := newChatSnapshots(h)
	repos, err := newRepositoriesContainer(
		ctx,
		adapters,
		h,
		chatSnapshots,
		axReviewThread,
		axWorkspace,
		axAgentChat,
		axAgentActivity,
		axAgentRunner,
		axNode,
		engines,
	)
	if err != nil {
		return nil, err
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.CrowbarHome()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	ucs, err := usecases.New(
		repos, toUsecaseStores(gormStores), engines, homeFunc, agentThreadBroadcast(h),
		announceHomeRow(h, chatSnapshots),
		announceRepoPlacement(h, gormStores.Repositories),
		usecases.WithChatSnapshots(chatSnapshots),
	)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}
	// The workspace-delete cascade purges each chat through the chat usecase's
	// own PurgeChat, which exists only now (it is built from repos); see
	// repositories.Container.PurgeChat.
	repos.PurgeChat = ucs.AgentChat.PurgeChat

	if err := startBootSweep(ctx, repos); err != nil {
		return nil, err
	}
	startProviderSweep(ctx, engines, repos, ucs)
	// A project or repo delete a crash or a failure stopped is finished before
	// anything is served (invariant D5); the tombstones it writes are purged by
	// the delete reactor like any other.
	if err := ucs.ProjectDelete.Resume(ctx); err != nil {
		slog.ErrorContext(ctx, "app: resume deletes (the next boot retries)", "err", err)
	}
	startRestoreTerminalSessions(ctx, ucs)
	reconcileAgentRunners(ctx, ucs)
	startOwningChatReconcile(ctx, repos, ucs)
	startTerminalWaitSweep(ctx, h, ucs)
	startModelDiscoveryWarmup(ctx, engines, crowbarHome)
	go descriptorcheck.LogAll(context.WithoutCancel(ctx), crowbarHome)

	rt := realtime.New(
		ctx,
		h,
		repos.Workspace,
		engines.Git,
		engines.FS,
		realtime.NewLSPLifecycle(engines.LSP),
		ucs.ProviderSync,
		poll.PerConnectionInterval,
		realtime.OriginSyncInterval,
		time.Now,
	)

	return &Container{
		Hub:             h,
		Repositories:    repos,
		GORM:            gormStores,
		Usecases:        ucs,
		Realtime:        rt,
		engines:         engines,
		axWorkspace:     axWorkspace,
		axReviewThread:  axReviewThread,
		axAgentChat:     axAgentChat,
		axAgentActivity: axAgentActivity,
		axAgentRunner:   axAgentRunner,
		axNode:          axNode,
	}, nil
}

// Shutdown gracefully quiesces the application layer's asynchronous machinery
// within the ctx deadline, BEFORE the adapter closes the DBs (spec §3.8 steps
// 2-3, decision 11). It runs ahead of Close in the ordered teardown, and its whole
// job is to leave every WRITER quiesced before the things they write to go away —
// in dependency order, outermost writer first:
//
//  1. quiesce the terminal engine: kill every live PTY and JOIN the exit callbacks
//     those deaths fire. This is FIRST, and it is not a resource release (Close
//     does that) — it is the outermost writer. A dying vendor CLI's exit callback
//     is the ONLY thing that records its death: it Exits the runner and closes the
//     turn the CLI abandoned. It runs on the terminal engine's reap goroutine, so
//     leaving it to Close — after steps 2-3 and the adapter's DB close — is a
//     RACE, and one that was lost in production ("close abandoned turn: get chat …
//     sql: database is closed"). Losing it is worse than a lost warning: the two
//     writes fall on opposite sides of the teardown, so the runner's Exit commits
//     while its turn is NOT closed — and with no live runner row left, the next
//     boot's ReconcileRunnersOnBoot has nothing to reconcile, so NOTHING ever
//     closes that turn. The chat spins forever, across every restart, and takes
//     the workspace's working overlay with it;
//  2. close the shared reactor drain gate (Cancel) so no post-commit reactor
//     wireCallbacks registered starts new work;
//  3. wait out the in-flight reactors, BOUNDED by ctx — a hung reactor cannot
//     wedge shutdown past the deadline (quiver's drainWg.Wait() is unbounded; we
//     bound it, decision 11);
//  4. Shutdown each per-type asynx singleton, draining its command/projection
//     pool (itself ctx-bounded) so no event is half-processed when the adapter
//     WAL-checkpoints and closes the event/read/view DBs. Step 1's writes are
//     folded into the read models here, which is the other half of why it must
//     precede this: an Exit sent AFTER an asynx Shutdown is simply rejected
//     ("asynx: shutting down") and the death goes unrecorded.
//
// Note what step 1 is NOT: it is not a second opinion about liveness. The PTY
// remains the sole authority — we kill the process and let its death carry the
// runner away, exactly as every other teardown path does. All we have changed is
// that the daemon now WAITS to hear the news before it dismantles the ears.
//
// Realtime resources (file watchers, LSP hosts) are released separately by Close.
// Every wait honors ctx, so the whole drain is bounded by the caller's deadline.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	// FIRST, before anything below can be cut short by ctx: this is the only
	// moment the agent tool counters are ever read.
	c.logAgentToolUsage()

	c.quiesceTerminal(ctx)

	drain := c.Repositories.Drain()
	drain.Cancel() // close the gate: reactors observing drainCtx stop starting new work.

	// Bounded by ctx: a stuck reactor delays shutdown by the deadline and no longer.
	//
	// The gate — not a bare WaitGroup — is what makes this safe. Step 1 above kills the
	// PTYs, and those deaths COMMIT EVENTS, which wake reactors: the daemon is at its most
	// eventful in the very instant it is trying to go quiet. A reactor Adding on asynx's
	// bus goroutine while this line Waits is textbook WaitGroup misuse, and `-race` caught
	// it. drain.Gate makes admitting work and beginning to drain one critical section.
	drain.Gate.Wait(ctx)

	// Order matters on the ABANDON path (the clean path already drained every reaper in
	// quiesceTerminal above, so all its writes have landed). If a reaper is still in flight
	// when we get here — quiesceTerminal was cut short by ctx — it writes runners.Exit
	// FIRST and chats.StopTurn SECOND (reconcileRunnerExit), and it RETURNS EARLY if the
	// first write is rejected. So shut the RUNNER store down before the CHAT store: a
	// reaper that loses the race then has its Exit rejected and bails before StopTurn,
	// leaving the runner row LIVE for the next boot's reconcile to retry both writes. The
	// reverse order lets Exit commit while StopTurn is rejected — runner gone, turn
	// stranded, and boot reconcile has no live row left to find. Recoverable vs permanent.
	return errors.Join(
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentRunner.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axAgentActivity.Shutdown(ctx),
		c.axNode.Shutdown(ctx),
	)
}

// logAgentToolUsage emits the one and only read of the agent capability
// surface's call counters, as a single line at shutdown:
//
//	agent tool usage this boot  tools="post_review_comment=7/1 set_chat_title=3/0"
//
// (tool=calls/failures, sorted, so consecutive boots diff cleanly.)
//
// The counters exist to settle whether agents actually USE these tools — the
// shell command this surface replaces is known to be ignored by real models —
// and a counter nothing reads settles nothing. Shutdown is the right and only
// place: the numbers are cumulative over a daemon's lifetime, so this is the
// moment they are complete. A boot that saw no tool call logs nothing rather
// than an empty line.
//
// Deliberately not an HTTP route. This is a diagnostic about the daemon, not a
// resource of the product, and nothing in the UI consumes it.
func (c *Container) logAgentToolUsage() {
	if c.Usecases == nil {
		return
	}
	stats := c.Usecases.AgentToolMetrics()
	if len(stats) == 0 {
		return
	}
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d/%d", name, stats[name].Calls, stats[name].Failures))
	}
	slog.Info("agent tool usage this boot (tool=calls/failures)", "tools", strings.Join(parts, " "))
}

// quiesceTerminal runs Shutdown's step 1 — kill the PTYs, join their exit callbacks
// — BOUNDED by ctx, mirroring how the reactor drain below it is bounded (decision
// 11): a reaper wedged on something we do not control must not hold the daemon past
// its deadline. The quiesce is latched in the engine container, so the later
// engines.Close() never re-enters (and so never re-wedges) whatever we abandon here.
func (c *Container) quiesceTerminal(
	ctx context.Context,
) {
	if c.engines == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.engines.QuiesceTerminal()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Close tears down the application layer's live realtime resources: it stops
// every file watcher and LSP host the service still holds, and — on whichever
// path reaches Close WITHOUT a preceding Shutdown (harness.crash's simulated
// death; a production Serve failure that returns before Run's ctx.Done branch
// ever runs) — stops the six per-type asynx singletons' own background worker
// pools too, so neither path leaks them into whatever the process does next.
// It is idempotent and runs on graceful shutdown so fsnotify file descriptors
// and LSP subprocesses are released promptly.
func (c *Container) Close() {
	shutdownAgentRunners(c.Usecases)
	c.Realtime.Close()
	c.stopBackgroundWorkers(context.Background())
}

// stopBackgroundWorkers stops each per-type asynx singleton's own worker/
// dispatcher goroutines (8 shards x 8 workers plus dispatchers apiece, spun up
// by asynx.Builder at construction — see newAsynx) WITHOUT running Shutdown's
// write-path steps (terminal quiesce, reactor drain): those exist to let a
// GRACEFUL stop record every in-flight death before the stores close, and
// Close running them again here would be meaningless at best (nothing is
// listening any more, the graceful path already ran them) and unsafe at worst
// on the paths that reach Close first (a stray write racing an adapter that
// may already be closed).
//
// A Shutdown that already ran for this Container makes every call here an
// immediate, harmless no-op (asynx.ErrAlreadyShuttingDown, swallowed) — each
// per-type Shutdown latches via its own CompareAndSwap, so Close can call this
// unconditionally instead of tracking whether Shutdown ran. On the path that
// DIDN'T run one (a crash, a Serve failure), this is what stops the shard
// pools rather than leaving them idling on an empty queue forever: they were
// the one background resource Close used to leave for the caller to leak.
func (c *Container) stopBackgroundWorkers(
	ctx context.Context,
) {
	_ = c.axWorkspace.Shutdown(ctx)
	_ = c.axReviewThread.Shutdown(ctx)
	_ = c.axAgentRunner.Shutdown(ctx)
	_ = c.axAgentChat.Shutdown(ctx)
	_ = c.axAgentActivity.Shutdown(ctx)
	_ = c.axNode.Shutdown(ctx)
}

// agentThreadBroadcast adapts the hub into the agentusecase.ToolThreadBroadcast seam:
// when an agent posts a review comment, the resulting thread has to reach a review
// pane that is already open, exactly as an HTTP-authored comment does.
//
// The conversion lives HERE, in the app layer, because it is the DTO boundary. The
// review-thread repository does not fan out (its store.BroadcastFunc is a no-op)
// and it cannot: the frame is built from domain.ReviewThread, which carries WsID but
// no project or repo id, while the /threads stream filters on all three. Only a
// caller holding the resolved workspace can supply them, so the aggregate crosses
// the usecase boundary and the DTO is assembled at the layer that owns wire types.
//
// This does NOT double-broadcast alongside the thread handler's own push: both end
// at the same ws.Broadcaster, but the agent path never runs the handler, and the
// handler's path never runs this.
func agentThreadBroadcast(
	h threadBroadcaster,
) agentusecase.ToolThreadBroadcast {
	return func(thread domain.ReviewThread, projectID, repoID string) {
		h.BroadcastThread(dto.ThreadDTOFrom(thread, projectID, repoID))
	}
}

// threadBroadcaster is the one hub method agentThreadBroadcast needs, narrowed to
// it so nothing else about the hub is in scope here. *hub.Hub satisfies it.
type threadBroadcaster interface {
	BroadcastThread(
		t dto.ThreadDTO,
	)
}

// newAxNode builds the Node position aggregate's per-type singleton, mirroring
// axAgentChat's own construction. Purely additive (2026-09-08
// sidebar-placement-unification, Task 1): nothing else sends Node commands
// yet — later tasks migrate existing placement logic onto it one vertical
// slice at a time. Split out of New only to keep that constructor within its
// length budget.
func newAxNode(
	adapters *adapter.Container,
) (asynx.Asynx[domain.Node], error) {
	axNode, err := newAsynx[domain.Node](adapters.NodeES(), adapters.NodeSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx node: %w", err)
	}
	return axNode, nil
}

// newRepositoriesContainer builds the repository layer from every per-type
// asynx singleton and the injected app-layer seams. The agent aggregates
// announce; the fanout built here decides what a client is told — the hub
// still reaches the repository layer for workspace frames, which are outside
// this subsystem. No live-update consumer is wired to Node yet (Task 1 is
// purely additive), so its watch is nil — safe, mirroring agentchat's own
// nil-tolerant hub projection. Split out of New only to keep that constructor
// within its length budget, mirroring newAgentWiring/newProjectImport in
// usecases/container.go.
func newRepositoriesContainer(
	ctx context.Context,
	adapters *adapter.Container,
	h *hub.Hub,
	chatSnapshots *agentusecase.ChatSnapshots,
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	axAgentChat asynx.Asynx[domain.Chat],
	axAgentActivity asynx.Asynx[domain.ChatActivity],
	axAgentRunner asynx.Asynx[agents.Runner],
	axNode asynx.Asynx[domain.Node],
	engines *engine.Container,
) (*repositories.Container, error) {
	agentFanout := agentusecase.NewFanout(chatSnapshots)
	repos, err := repositories.New(
		ctx,
		adapters,
		h,
		axReviewThread,
		axWorkspace,
		axAgentChat,
		axAgentActivity,
		axAgentRunner,
		axNode,
		engines.Git,
		agentFanout.ChatWatch(),
		agentFanout.RunnerWatch(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}
	return repos, nil
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:                 gormStores.Projects,
		Repositories:             gormStores.Repositories,
		TerminalProfiles:         gormStores.TerminalProfiles,
		TerminalSessions:         gormStores.TerminalSessions,
		AgentProviderPreferences: gormStores.AgentProviderPreferences,
		AgentPermissionDefault:   gormStores.AgentPermissionDefault,
		AgentModelManifestFetch:  gormStores.AgentModelManifestFetch,
		Folders:                  gormStores.Folders,
		AgentChatTelemetry:       gormStores.AgentChatTelemetry,
	}
}

// newChatSnapshots builds the one owner of every chat's versioned snapshot and
// binds its frames to the hub. Built before the repositories, because their
// watch seams feed it; handed to the chat usecase, which binds its read model.
func newChatSnapshots(h *hub.Hub) *agentusecase.ChatSnapshots {
	snaps := agentusecase.NewChatSnapshots()
	snaps.SetPublish(func(f agentusecase.ChatSnapshotFrame) {
		h.BroadcastAgentChatEvent(chatSnapshotEvent(f))
	})
	return snaps
}

// chatSnapshotEvent is one snapshot frame on the wire: the chat's full DTO
// (worktree state rides its own worktree_state frame) and the kind of the
// event that produced it.
func chatSnapshotEvent(f agentusecase.ChatSnapshotFrame) dto.AgentChatEvent {
	s := f.Snapshot
	ev := dto.AgentChatEvent{
		ChatID:      s.Chat.ID,
		WorkspaceID: s.Chat.WorkspaceID,
		Kind:        f.Kind,
		RunnerID:    f.RunnerID,
		Version:     s.Version,
	}
	if f.Deleted {
		return ev
	}
	chat := dto.AgentChatDTOFrom(s.Chat, dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
		s.TerminalWait, s.AttachedSessionID, s.Session), nil)
	ev.Chat = &chat
	return ev
}

// startTerminalWaitSweep begins the cadence that notices a vendor CLI parked on a
// modal Crowbar cannot answer — the workspace-trust dialog and its relatives, which
// reach the daemon through no hook and otherwise leave a chat pane showing nothing
// at all over a live, blocked process.
//
// Started HERE, beside the boot reconcile, rather than inside the usecase: the
// detector publishes, and the thing it publishes through is the hub, which is a
// layer above every usecase. It is also why the sweep begins after
// reconcileAgentRunners — a restart's first census should be of runners already
// reconciled, not of rows the reconcile is about to retire.
func startTerminalWaitSweep(
	ctx context.Context,
	h *hub.Hub,
	ucs *usecases.Container,
) {
	ucs.AgentRunner.StartTerminalWaitSweep(ctx, agentusecase.ChatFeed{
		PromptSettled: h.BroadcastAgentChatPromptSettled,
		MessageDelta:  h.BroadcastAgentChatMessageDelta,
		Compaction:    h.BroadcastAgentChatCompaction,
		Plan:          h.BroadcastAgentChatPlan,
		Telemetry:     h.BroadcastAgentChatTelemetry,
	})
}

// startModelDiscoveryWarmup kicks a model.discover: source's live probe for
// every descriptor that declares one, at boot rather than waiting for the
// frontend's first providers request to trigger it lazily. List() already
// forks each descriptor's refresh instead of blocking on it (Cache.Refresh),
// so this call returns quickly; run off the request path anyway (its own
// goroutine) so a slow descriptor scan never adds to daemon startup time.
// Best-effort and fire-and-forget: List's error, if any, is left for the
// first real request to surface. This List call itself is detached from ctx
// (WithoutCancel, same as this file's other boot-time reconciliation calls)
// so an early cancellation never aborts the scan — but the refreshes it
// forks are NOT: engines was built from this same ctx (engine.New ->
// engineagents.WithLifecycle), so the daemon's own shutdown still stops
// every in-flight and future probe/fetch, request-triggered or not.
func startModelDiscoveryWarmup(
	ctx context.Context,
	engines *engine.Container,
	crowbarHome string,
) {
	if engines == nil || engines.Agents == nil {
		return
	}
	go func() {
		_, _ = engines.Agents.List(context.WithoutCancel(ctx), crowbarHome)
	}()
}

func startProviderSweep(
	ctx context.Context,
	engines *engine.Container,
	repos *repositories.Container,
	ucs *usecases.Container,
) {
	engines.Provider.StartBackgroundSweep(
		ctx,
		sweepTargets(repos.Workspace),
		sweepCallback(ctx, ucs),
	)
}

// startBootSweep runs the cheap, proactive boot orphan-sweep (spec §3.8)
// SYNCHRONOUSLY before app.New returns (and thus before internal.Run serves). It
// reaps every workspace left tombstoned by a delete whose reactor never finished
// — a crash mid-purge, or the drain gate refusing it at shutdown — by re-driving
// the SAME Purger the delete reactor runs (repositories/workspace, spec §7-D):
// the dependents cascade, the hardened root removal from the tombstone's own
// WorktreePath, then the aggregate Forget. A missing purger is a wiring error and
// is surfaced; per-row recovery failures are logged and never fail boot.
func startBootSweep(
	ctx context.Context,
	repos *repositories.Container,
) error {
	sweeper, ok := repos.Workspace.(workspace.BootSweeper)
	if !ok {
		return nil
	}
	sweeper.BackfillProvisioning(ctx)
	if err := sweeper.Sweep(ctx); err != nil {
		return fmt.Errorf("app: boot sweep: %w", err)
	}
	return nil
}

// startRestoreTerminalSessions reloads persisted terminal sessions as PTY-less
// placeholders so a subsequent client Attach transparently restores them.
// FIX 3: runs SYNCHRONOUSLY before the engine/HTTP layer starts serving, so
// the registry is fully populated before the first Attach can arrive. Running
// it in the background allowed concurrent Attach + restore races. Best-effort:
// per-row errors are logged; orphaned rows are reconciled automatically.
func startRestoreTerminalSessions(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.Terminal == nil {
		return
	}
	_ = ucs.Terminal.RestorePersistedSessions(context.WithoutCancel(ctx))
}

// reconcileAgentRunners reaps the live-runner rows of the previous run: agent_runners is
// durable sqlite and is never truncated at boot, but a PTY never survives a restart, so
// every row the daemon comes back to describes a CLI that no longer exists. Nothing
// recorded those deaths — the only thing that ever does is an exit callback that lived in
// the process that went away.
//
// A stale row is not cosmetic: it is indistinguishable from a running CLI to every read in
// the agent package, so it BRICKS the chat it names (ResumeChat finds it and no-ops, and
// the pane attaches to a dead terminal session) and, if the chat was mid-turn, spins its
// spinner forever. This runs on EVERY boot, so those states last until this call, not
// until the user gives up.
//
// It runs SYNCHRONOUSLY and AFTER startRestoreTerminalSessions. Synchronously matters: the
// first client read must not race it, or an HTTP read that beats the reconcile is served a
// corpse. The ordering relative to the restore does NOT: the reconcile asks SessionLive,
// which questions the PTY directly, not the terminal registry, so a not-yet-restored session
// and a restored PTY-less placeholder both read false — correctly, since a restored session
// is never live either way. (The hazard that WOULD make this ordering load-bearing belongs
// to the engine's SessionExists, not SessionLive: SessionExists answers true for every
// restored placeholder, which is exactly why ReconcileRunnersOnBoot insists on SessionLive
// instead — see its call site in agent.go.)
//
// Best-effort: a failure is logged and the daemon still boots. The chats it could not
// reconcile are no worse off than they were a line earlier, and refusing to start the
// daemon over them would be strictly worse.
//
// (Its sibling, seedAgentRegistry, is NOT coming back: it rehydrated an in-memory
// session→chat index from persisted segments, and that index is now the runner aggregate's
// append-only conversation history — already durable, and answering ChatForSession straight
// from the read model.)
func reconcileAgentRunners(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.AgentRunner == nil {
		return
	}
	if err := ucs.AgentRunner.ReconcileRunnersOnBoot(context.WithoutCancel(ctx)); err != nil {
		slog.WarnContext(ctx, "app: reconcile agent runners on boot", "err", err)
	}
}

// startOwningChatReconcile runs the D4 boot reconcile (owning_chats_reconcile.go)
// SYNCHRONOUSLY, before anything is served. A listing failure is logged: the
// daemon still boots, and the next boot tries again.
func startOwningChatReconcile(
	ctx context.Context,
	repos *repositories.Container,
	ucs *usecases.Container,
) {
	if ucs.AgentChat == nil || ucs.AgentChatFolder == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: boot: list workspaces for the owning-chat reconcile", "err", err)
		return
	}
	chats, err := ucs.AgentChat.ListChats(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: boot: list chats for the owning-chat reconcile", "err", err)
		return
	}
	reconcileOwningChats(ctx, workspaces, chats, ucs.AgentChatFolder)
}

// shutdownAgentRunners kills every live api-transport connection before the
// daemon exits. It is the shutdown-time mirror of reconcileAgentRunners:
// nothing else in Close's chain (engine.Container.Close, Realtime.Close)
// reaches this registry, so without this call every codex-style serve
// process outlives the daemon that spawned it.
func shutdownAgentRunners(ucs *usecases.Container) {
	if ucs.AgentRunner == nil {
		return
	}
	ucs.AgentRunner.ShutdownAPIConnections()
}

func sweepCallback(
	ctx context.Context,
	ucs *usecases.Container,
) func(
	wsID string,
	state provider.ProviderState,
) {
	return func(
		wsID string,
		state provider.ProviderState,
	) {
		_ = ucs.ProviderSync.SyncFromState(
			context.WithoutCancel(ctx),
			wsID,
			state,
			time.Now(),
		)
	}
}

// shouldSweep reports whether the global cron should re-poll a workspace: it
// must have a live PR (PRUrl != "") and must not be in a terminal PR state
// (pr-merged/pr-closed), which are never re-polled (D10/§11). This widens the
// old Status==pr-open filter so pr-open->pr-merged/closed transitions are
// observed on unwatched workspaces and pr-conflicts workspaces keep syncing.
func shouldSweep(
	ws domain.Workspace,
) bool {
	if ws.PRUrl == "" {
		return false
	}
	if ws.Status == domain.WorkspaceStatusPRMerged ||
		ws.Status == domain.WorkspaceStatusPRClosed {
		return false
	}
	return true
}

func sweepTargets(
	repo workspace.Workspace,
) func() []poll.SweepTarget {
	return func() []poll.SweepTarget {
		rows, err := repo.List(context.Background())
		if err != nil {
			return nil
		}
		targets := make([]poll.SweepTarget, 0, len(rows))
		for _, ws := range rows {
			if !shouldSweep(ws) {
				continue
			}
			targets = append(targets, poll.SweepTarget{
				WSID:      ws.ID,
				RepoPath:  ws.WorktreePath,
				Branch:    ws.Branch,
				HasOpenPR: true,
			})
		}
		return targets
	}
}

// announceHomeRow fans a home row a repo drag shifted out on the chats WS
// by its own kind: a chat frame names the chat, a folder frame the folder.
func announceHomeRow(
	h *hub.Hub,
	chatSnapshots *agentusecase.ChatSnapshots,
) project.HomeRowAnnouncer {
	return func(ctx context.Context, id, workspaceID string, kind domain.NodeKind, event string) {
		if kind == domain.NodeKindChat {
			// A versioned snapshot, like every other chat frame: its placement
			// is read from the Node this write just moved.
			chatSnapshots.Announce(ctx, id, event)
			return
		}
		h.BroadcastAgentChatFolder(id, workspaceID, event)
	}
}

// announceRepoPlacement fans a repo header row's DECIDED placement out as a
// RepoDTO — for a repo the chat tree renumbered as collateral of a chat or
// folder drag, whose Node write no projection announces.
func announceRepoPlacement(
	h *hub.Hub,
	repos store.ScopedStore[domain.Repository, string],
) func(ctx context.Context, repoID, parentID string, order int) {
	return func(ctx context.Context, repoID, parentID string, order int) {
		repo, err := repos.FindByKey(ctx, repoID)
		if err != nil || repo == nil {
			return
		}
		h.BroadcastRepo(dto.RepoDTOFrom(*repo, dto.RepoPlacement{FolderID: parentID, Order: order}))
	}
}
