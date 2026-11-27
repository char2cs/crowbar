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

type Container struct {
	Workspace           workspace.Workspace
	ReviewThread        reviewthread.ReviewThread
	ReviewThreadChanges *reviewthread.Changes
	AgentChat           agentchat.EventStore
	Placement           *repotree.Ports
	ChatLedger          *ledgerstore.Store
	ChatBlobs           *ledgerstore.Blobs
	publishWorkspace    func(w dto.WorkspaceDTO, rev int64)
	revs                atomic.Int64
	git                 wsusecase.MergeConflictBatchChecker
	PurgeChat           func(ctx context.Context, chatID string) error
	axWorkspace         asynx.Asynx[domain.Workspace]
	axReviewThread      asynx.Asynx[domain.ReviewThread]
	axAgentChat         asynx.Asynx[domain.Chat]

	mu              sync.Mutex
	agentWorking    map[string]map[string]struct{}
	agentBroadcasts map[string]bool

	drainGate   *drain.Gate
	drainCtx    context.Context
	drainCancel context.CancelFunc
}

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
	if err := workspace.RegisterHubProjection(c.Workspace, c.revFrame, func(f revFrame) {
		c.publishWorkspace(f.row, f.rev)
	}); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	threadChanges := reviewthread.NewChanges()
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), rm.ReviewThread, threadChanges.Notify)
	if err != nil {
		return nil, err
	}
	c.ReviewThread = rt
	c.ReviewThreadChanges = threadChanges

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
	if c.ChatBlobs, err = ledgerstore.OpenBlobs(adapters.Home().Content()); err != nil {
		return nil, fmt.Errorf("repositories: chat payloads: %w", err)
	}

	if err := c.wireCallbacks(ctx, adapters.Home().Root(), onGiveUp); err != nil {
		return nil, fmt.Errorf("repositories: wire callbacks: %w", err)
	}
	return c, nil
}

func (c *Container) WaitQuiescent() {
	c.axWorkspace.WaitPublish()
	c.axReviewThread.WaitPublish()
	c.axAgentChat.WaitPublish()
	c.Placement.WaitPublish()
}

func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	return c.enrichFrameHearing(ctx, ws, nil)
}

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
	owner, owned := domain.ResolveOwningChat(heard.over(rows, ws.ID))
	if !owned {
		return owner, false
	}
	return c.atItsNode(ctx, owner), true
}

func (c *Container) WorkingFor(
	wsID string,
) bool {
	return c.agentWorkingFor(wsID)
}

func (c *Container) Republish(
	ctx context.Context,
	wsID string,
) {
	c.republishHearing(ctx, wsID, nil)
}

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
