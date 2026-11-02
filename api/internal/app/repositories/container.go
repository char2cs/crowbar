package repositories

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
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
	// Placement serves every sidebar row's position, each folder's identity and
	// each chat's own placement, by id, over the per-project Tree aggregates.
	Placement *repotree.Ports
	// ChatLedger is each chat's append-only ledger, one writer per chat.
	ChatLedger *ledgerstore.Store
	// ChatBlobs holds the tool payloads the ledger's rows name.
	ChatBlobs *ledgerstore.Blobs
	// publishWorkspace sends a workspace's enriched row to the projects scope at
	// rev, taken before the row was read (see nextRev).
	publishWorkspace func(w dto.WorkspaceDTO, rev int64)
	// revs is the last rev a workspace row was read at.
	revs atomic.Int64
	git  wsusecase.MergeConflictBatchChecker
	// PurgeChat hard-deletes one chat and everything it owns — its aggregate, the
	// CLIs on it, its conversation record and telemetry, its conversation history
	// and its ledger directory. It is the chat usecase's own PurgeChat, the SAME
	// path a user's chat delete takes, so the workspace-delete cascade cannot
	// drift from it again (spec §7-D, invariant D5): it used to keep its own copy
	// that skipped the conversation record, the telemetry and the ledger.
	//
	// Assigned after construction because the chat usecase is built from this
	// container. The cascade REFUSES to run without it rather than purging
	// half a chat.
	PurgeChat func(ctx context.Context, chatID string) error
	// axWorkspace/axReviewThread/axAgentChat are the per-type asynx
	// instances WaitQuiescent drains: the read-your-writes barrier for tests.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.Chat]

	mu sync.Mutex
	// agentWorking maps a workspace id to its busy agent chats; agentBroadcasts
	// is each workspace's rebroadcast worker (agent_working.go). Guarded by mu.
	agentWorking    map[string]map[string]struct{}
	agentBroadcasts map[string]bool

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

// New builds all aggregate repositories, wiring the workspace projection into
// publishWorkspace. The workspace aggregate is backed by the singleton axWorkspace (one
// instance per type, routing every id by shard hash) built by the app layer; its
// read model lives in state/store/workspace.db.
// The reviewthread aggregate owns its central per-type read model at
// state/store/review_thread.db. The agentchat aggregate owns its central
// per-type read model at state/store/agent_chat.db. The workspace-delete
// cascade's chat purge, Container.PurgeChat, is assigned by the app layer once
// the chat usecase exists; see the field's doc comment. rm holds the read
// models the asynx instances were built to deliver to (OpenReadModels).
func New(
	ctx context.Context,
	adapters *adapter.Container,
	rm *ReadModels,
	publishWorkspace func(w dto.WorkspaceDTO, rev int64),
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	axAgentChat asynx.Asynx[domain.Chat],
	placement *repotree.Ports,
	git wsusecase.MergeConflictBatchChecker,
	chatWatch agentchat.WatchFunc,
	onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
) (*Container, error) {
	c := &Container{
		publishWorkspace: publishWorkspace, git: git,
		agentWorking: map[string]map[string]struct{}{}, agentBroadcasts: map[string]bool{},
		axWorkspace: axWorkspace, axReviewThread: axReviewThread, axAgentChat: axAgentChat,
		Placement: placement,
	}
	ws, err := workspace.New(axWorkspace, adapters.WorkspaceES(), rm.Workspace)
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	// Every event-driven workspace row goes through the SAME enrichFrame as the
	// agent-turn republishes, so its Working and merge fields agree with them.
	if err := workspace.RegisterHubProjection(c.Workspace, c.revFrame, func(f revFrame) {
		c.publishWorkspace(f.row, f.rev)
	}); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	// reviewthread owns its own central per-type read model at
	// state/store/review_thread.db (Task 12), no longer the shared view.db: pass
	// ReviewThreadView() as the read-model DB while keeping ReviewThreadES() for the
	// lazy AggregateLister Replay (§3.7).
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), rm.ReviewThread, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.ReviewThread = rt

	// agentchat: build the asynx-backed EventStore over the singleton
	// axAgentChat, registering its store + hub projections (store.New, invoked by
	// NewEventSourced) exactly once. chatWatch is the SOLE source of
	// agent-chat lifecycle frames: every agentchat.* event the agent usecase's
	// commands emit is fanned out here by the hub projection. The usecase no
	// longer broadcasts manually (that double-broadcast was retired at cutover),
	// so this projection is the one and only WS feed for agent chats.
	agentChat, err := agentchat.NewEventSourced(ctx, axAgentChat, adapters.AgentChatES(), rm.AgentChat, chatWatch, placement)
	if err != nil {
		return nil, fmt.Errorf("repositories: agent chat event store: %w", err)
	}
	c.AgentChat = agentChat
	if _, err := axAgentChat.OnForget(c.forgetAgentWorking); err != nil {
		return nil, fmt.Errorf("repositories: agent working overlay: %w", err)
	}
	if _, err := axAgentChat.Subscribe(asynx.Topic("agentchat.workspace_set.*"), c.ownerMoved); err != nil {
		return nil, fmt.Errorf("repositories: workspace owner: %w", err)
	}

	if c.ChatLedger, err = ledgerstore.Open(ctx, adapters.ChatLedgerDB(), time.Now); err != nil {
		return nil, fmt.Errorf("repositories: chat ledger: %w", err)
	}
	// Tool payloads live beside the state directory, swept by its retention.
	if c.ChatBlobs, err = ledgerstore.OpenBlobs(adapters.Home().Content()); err != nil {
		return nil, fmt.Errorf("repositories: chat payloads: %w", err)
	}

	// Wire the post-commit cross-aggregate reactions (spec §3.6): the workspace
	// delete reactor + its review-thread AND agent-chat forget cascades, all
	// joined to the shared drain WaitGroup so graceful shutdown can quiesce them
	// (Task 15). Every cascaded repo must already be built — forgetDependents
	// calls into c.ReviewThread and c.AgentChat.
	if err := c.wireCallbacks(ctx, adapters.Home().Root(), onGiveUp); err != nil {
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
	c.Placement.WaitPublish()
}

// enrichFrame adds the overlays the aggregate lacks — Working (an agent chat
// mid-turn) and merge eligibility (spec §10) — in the ONE place the hub
// projection and the agent-turn rebroadcasts share, so their frames agree.
func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	return c.enrichFrameHearing(ctx, ws, nil)
}

// enrichFrameHearing is enrichFrame with heard folded over ws's chats as the
// read model lists them; nil hears nothing.
func (c *Container) enrichFrameHearing(
	ctx context.Context,
	ws domain.Workspace,
	heard *heardChat,
) dto.WorkspaceDTO {
	ws.Working = c.WorkingFor(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	owner, owned := c.resolveOwningChat(ctx, ws, heard)
	return dto.WorkspaceDTOFrom(ctx, ws, elig, owner.ID, c.nodePlacement(ws, owner, owned))
}

// nodePlacement is ws's own sidebar placement for the wire DTO: an ordinary
// fork sits where its owning chat does; only a locked branch, whose owning
// chat has no Node, is placed by ws.ID's own workspace Node.
func (c *Container) nodePlacement(
	ws domain.Workspace,
	owner domain.Chat,
	owned bool,
) dto.WorkspacePlacementReader {
	if !ws.RendersAsBranch() && owned {
		return chatPlacementReader{parentID: owner.ParentID, order: owner.Order}
	}
	if c.Placement == nil {
		return nil
	}
	return nodePlacementReader{nodes: c.Placement, nodeID: ws.ID}
}

// chatPlacementReader answers a resolved owning chat's own ParentID/Order
// directly — no store read of its own, since the caller already resolved
// the chat this frame needs.
type chatPlacementReader struct {
	parentID string
	order    int
}

func (r chatPlacementReader) Placement(
	_ context.Context,
	_ string,
) (folderID string, order int) {
	return r.parentID, r.order
}

type nodePlacementReader struct {
	nodes  *repotree.Ports
	nodeID string
}

func (r nodePlacementReader) Placement(
	ctx context.Context,
	_ string,
) (folderID string, order int) {
	n, err := r.nodes.GetNode(ctx, r.nodeID)
	if err != nil {
		return "", 0
	}
	return n.ParentID, n.Order
}

// resolveOwningChat resolves ws's real owning chat ROW (not just its id),
// reusing domain.ResolveOwningChat over this container's own AgentChat read —
// never a second, independently derived answer. An unwired AgentChat (a test
// Container built with only the fields its own assertion needs) or an
// unresolvable read degrades to no owner.
func (c *Container) resolveOwningChat(
	ctx context.Context,
	ws domain.Workspace,
	heard *heardChat,
) (domain.Chat, bool) {
	if c.AgentChat == nil {
		return domain.Chat{}, false
	}
	rows, err := c.AgentChat.ListByWorkspace(ctx, ws.ID)
	if err != nil {
		return domain.Chat{}, false
	}
	return domain.ResolveOwningChat(heard.over(rows, ws.ID))
}

// WorkingFor reports whether an agent chat in the workspace is mid-turn. The
// REST reads, the snapshots and the live frames all stamp Working from it; work
// an operation runs on the workspace is the operation's own state.
func (c *Container) WorkingFor(
	wsID string,
) bool {
	return c.agentWorkingFor(wsID)
}

// Republish pushes the workspace's current row to the hub for a change no
// workspace event carries (an agent turn, a move of its Node). A failed read
// is logged: nothing repeats the flip it carried.
func (c *Container) Republish(
	ctx context.Context,
	wsID string,
) {
	c.republishHearing(ctx, wsID, nil)
}

// eligibilityFor resolves the merge-eligibility overlay (incl. the predicted
// merge-conflict flag) for ws by reading its siblings and delegating to the
// shared wsusecase.ResolveMergeEligibilities — the SAME resolver the snapshot read
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
	return wsusecase.ResolveMergeEligibilities(ctx, []domain.Workspace{ws}, siblings, c.git)[0]
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
