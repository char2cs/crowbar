package repositories

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the four aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	AgentRun     agentrun.AgentRun
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. Read models live in the shared GORM DB.
func New(
	db *gormdb.DB,
	h hub.WebSocketHub,
	axWorkspace asynx.Asynx[domain.Workspace],
	axChat asynx.Asynx[domain.Chat],
	axAgentRun asynx.Asynx[domain.AgentRun],
	axReviewThread asynx.Asynx[domain.ReviewThread],
) (*Container, error) {
	c := &Container{hub: h}
	ws, err := workspace.New(axWorkspace, db, func(w domain.Workspace) {
		c.broadcastWorkspace(context.Background(), w)
	})
	if err != nil {
		return nil, err
	}
	ch, err := chat.New(axChat, db, broadcastChat(h))
	if err != nil {
		return nil, err
	}
	ar, err := agentrun.New(axAgentRun, db, func(domain.AgentRun) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(axReviewThread, db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.Chat = ch
	c.AgentRun = ar
	c.ReviewThread = rt
	return c, nil
}

func broadcastChat(
	h hub.WebSocketHub,
) chat.BroadcastFunc {
	return func(c domain.Chat) {
		h.BroadcastChat(hub.ChatStatusEvent{ChatID: c.ID, WsID: c.WsID, Status: c.Status})
	}
}

// RegisterHubProjections wires the AgentRun subscription that drives Chat status
// and the Workspace agent-running overlay (03 §7). Workspace and Chat broadcast
// directly from their read-model projections (built in New).
func (c *Container) RegisterHubProjections(
	axAgentRun asynx.Asynx[domain.AgentRun],
) error {
	return RegisterAgentRunProjection(axAgentRun, c.Chat, c.AgentRun, c.refreshWorkspace)
}

func (c *Container) refreshWorkspace(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	ws.AgentRunning = c.hasLiveAgentRun(ctx, ws.ID)
	c.hub.BroadcastWorkspace(ws)
}

// ListWorkspacesWithOverlay returns every workspace row with the derived
// agent-running overlay computed at call time, matching the live broadcast path
// (00 §6.1, 03 §1a). The persisted HasConflicts field is left as stored. It backs
// the Workspaces snapshot-on-subscribe so a client connecting mid-agent-run sees
// the spinner immediately.
func (c *Container) ListWorkspacesWithOverlay(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces with overlay: %w", err)
	}
	running := c.runningWorkspaceIDs(ctx)
	for i := range rows {
		rows[i].AgentRunning = running[rows[i].ID]
	}
	return rows, nil
}

func (c *Container) hasLiveAgentRun(
	ctx context.Context,
	wsID string,
) bool {
	return c.runningWorkspaceIDs(ctx)[wsID]
}

func (c *Container) runningWorkspaceIDs(
	ctx context.Context,
) map[string]bool {
	ids := map[string]bool{}
	if c.AgentRun == nil {
		return ids
	}
	running, err := c.AgentRun.ListRunning(ctx)
	if err != nil {
		return ids
	}
	for _, run := range running {
		ids[run.WsID] = true
	}
	return ids
}

// RecoverOrphans runs AgentRun crash recovery (running→error) then the idempotent
// chat reconcile, in order, both draining via SendWait (00 §6.2).
func (c *Container) RecoverOrphans(
	ctx context.Context,
) {
	RecoverAgentRuns(ctx, c.AgentRun)
	ReconcileChats(ctx, c.Chat, c.AgentRun)
}
