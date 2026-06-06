package repositories

import (
	"context"

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
	ws, err := workspace.New(axWorkspace, db, h.BroadcastWorkspace)
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
	return &Container{Workspace: ws, Chat: ch, AgentRun: ar, ReviewThread: rt, hub: h}, nil
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
	c.hub.BroadcastWorkspace(ws)
}

// RecoverOrphans runs AgentRun crash recovery (running→error) then the idempotent
// chat reconcile, in order, both draining via SendWait (00 §6.2).
func (c *Container) RecoverOrphans(
	ctx context.Context,
) {
	RecoverAgentRuns(ctx, c.AgentRun)
	ReconcileChats(ctx, c.Chat, c.AgentRun)
}
