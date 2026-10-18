package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
	"github.com/char2cs/crowbar/api/internal/core/migrate"
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
	crowbarHome  string

	// engines is retained for ONE reason: Shutdown must quiesce the terminal engine
	// (kill every PTY and JOIN the exit callbacks those deaths fire) BEFORE it drains
	// the aggregates below, because those callbacks are writers into them. See
	// Shutdown step 1.
	engines *engine.Container

	// axWorkspace, axReviewThread and axAgentChat are the per-type asynx
	// singletons, kept so the ordered graceful shutdown can drain each.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.Chat]
	// axAgentActivity is the conversation record's own per-type singleton. It is
	// separate from axAgentChat because their write rates differ by orders of
	// magnitude: a chat emits a handful of events, its activity emits hundreds per
	// turn, and sharing one single-writer event log would put a sidebar repaint
	// behind a tool-call storm.
	axAgentActivity asynx.Asynx[domain.ChatActivity]
	// axNode is the Node position aggregate's own per-type singleton
	// (2026-09-08 sidebar-placement-unification): the ONE entity that will own
	// every sidebar row's ParentID/Order. This wiring is purely additive —
	// nothing else in the codebase reads or writes it yet.
	axNode asynx.Asynx[domain.Node]
}

// New constructs the application layer from the engine and adapter containers,
// wires the aggregate repositories into the hub (00 §7) and runs post's steps.
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
	post migrate.PostOpenRunner,
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
	// axAgentChat is the per-type singleton over state/events/agent_chat.db; its
	// projections are registered via repositories.New -> agentchat.NewEventSourced.
	axAgentChat, err := newAsynx[domain.Chat](adapters.AgentChatES(), adapters.AgentChatSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent chat: %w", err)
	}
	axAgentActivity, err := newAsynx[domain.ChatActivity](
		adapters.AgentActivityES(), adapters.AgentActivitySS(),
	)
	if err != nil {
		return nil, fmt.Errorf("app container: agent activity asynx: %w", err)
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
	choiceWatch := agentusecase.NewChoiceWatch()
	repos, err := newRepositoriesContainer(
		ctx,
		adapters,
		h,
		chatSnapshots,
		choiceWatch,
		axReviewThread,
		axWorkspace,
		axAgentChat,
		axAgentActivity,
		axNode,
		engines,
	)
	if err != nil {
		return nil, err
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.Home().Root()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	ucs, err := usecases.New(
		repos, toUsecaseStores(gormStores), engines, homeFunc, agentThreadBroadcast(h),
		announceHomeRow(h, chatSnapshots),
		announceRepoPlacement(h, gormStores.Repositories),
		usecases.WithChatSnapshots(chatSnapshots),
		usecases.WithChoiceWatch(choiceWatch),
	)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}
	// The workspace-delete cascade purges each chat through the chat usecase's
	// own PurgeChat, which exists only now (it is built from repos); see
	// repositories.Container.PurgeChat.
	repos.PurgeChat = ucs.AgentChat.PurgeChat
	c := &Container{
		Hub:             h,
		Repositories:    repos,
		GORM:            gormStores,
		Usecases:        ucs,
		crowbarHome:     crowbarHome,
		engines:         engines,
		axWorkspace:     axWorkspace,
		axReviewThread:  axReviewThread,
		axAgentChat:     axAgentChat,
		axAgentActivity: axAgentActivity,
		axNode:          axNode,
	}

	if err := startBootSweep(ctx, repos); err != nil {
		return nil, c.abandon(ctx, err)
	}
	// A project or repo delete a crash or a failure stopped is finished before
	// anything is served (invariant D5); the tombstones it writes are purged by
	// the delete reactor like any other.
	if err := ucs.ProjectDelete.Resume(ctx); err != nil {
		slog.ErrorContext(ctx, "app: resume deletes (the next boot retries)", "err", err)
	}
	startRestoreTerminalSessions(ctx, ucs)
	// Before the owner reconcile (a project home created here needs an owner).
	if err := post.RunPostOpen(ctx, legacyImpls(newLegacyFuncs(repos, ucs, adapters.Home()))); err != nil {
		return nil, c.abandon(ctx, fmt.Errorf("app: migrate: %w", err))
	}
	// After the upgrade steps, which can hand it a runner a crashed old daemon left.
	reconcileAgentRunners(ctx, ucs)
	// Loops start once the boot can no longer fail, so a failed one leaves none.
	startProviderSweep(ctx, engines, repos, ucs)
	startOwningChatReconcile(ctx, repos, ucs)
	startTerminalWaitSweep(ctx, h, ucs)
	startModelDiscoveryWarmup(ctx, engines, crowbarHome)
	go descriptorcheck.LogAll(context.WithoutCancel(ctx), crowbarHome)

	c.Realtime = realtime.New(
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
	return c, nil
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
// this subsystem. Node's live-update watch is nil; its saved placements reach
// the hub with the chats' through BroadcastPlacementChanged. Split out of New only to keep that constructor
// within its length budget, mirroring newAgentWiring/newProjectImport in
// usecases/container.go.
func newRepositoriesContainer(
	ctx context.Context,
	adapters *adapter.Container,
	h *hub.Hub,
	chatSnapshots *agentusecase.ChatSnapshots,
	choiceWatch *agentusecase.ChoiceWatch,
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	axAgentChat asynx.Asynx[domain.Chat],
	axAgentActivity asynx.Asynx[domain.ChatActivity],
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
		axNode,
		engines.Git,
		agentFanout.ChatWatch(),
		nil,
		choiceWatch.Notify,
		func(string) { h.BroadcastPlacementChanged() },
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
	conversation, _ := s.Conversation()
	chat := dto.AgentChatDTOFrom(s.Chat, dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
		s.TerminalWait, s.AttachedSessionID, s.Session, conversation), nil)
	ev.Chat = &chat
	return ev
}

// broadcastChoices publishes a chat's pending prompts as the wire frame.
func broadcastChoices(h *hub.Hub) func(string, string, []domain.ActivityChoice, []string) {
	return func(chatID, workspaceID string, choices []domain.ActivityChoice, answerable []string) {
		h.BroadcastAgentChatChoices(chatID, workspaceID, dto.AgentChoiceDTOsFrom(choices, answerable))
	}
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
		Choices:       broadcastChoices(h),
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

// reconcileAgentRunners ends the runners the ledger still holds open from the
// previous run: their PTYs died with it, and left open one keeps its chat busy.
// It runs before anything is served, so no client read races it.
//
// Best-effort: a failure is logged and the daemon still boots; those chats are no
// worse off than before, and refusing to start would be strictly worse.
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
