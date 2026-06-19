package repositories

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
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
	rt, err := reviewthread.New(axReviewThread, db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.Chat = ch
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

// broadcastWorkspace pushes a workspace row to the hub. The derived Working
// overlay is always false: the agent-run concept has been removed and the chat
// usecase that would set it is a dormant TODO (00 §5).
func (c *Container) broadcastWorkspace(
	_ context.Context,
	ws domain.Workspace,
) {
	ws.Working = false
	c.hub.BroadcastWorkspace(ws)
}

// ListWorkspaces returns every workspace row. The working overlay has been
// removed (00 §5); Working is always false until the chat usecase is
// implemented. It backs the Workspaces snapshot-on-subscribe.
func (c *Container) ListWorkspaces(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces: %w", err)
	}
	for i := range rows {
		rows[i].Working = false
	}
	return rows, nil
}
