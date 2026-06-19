package repositories

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
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
// the hub. The workspace aggregate is per-entity event-sourced: its Asynx
// instances and view DBs are resolved lazily from the adapter container by ID,
// using the injected asynxFactory (passed by the app layer to avoid an import
// cycle on newAsynx). The chat and reviewthread aggregates keep their global
// event stores and read models in the global view DB.
func New(
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	asynxFactory workspace.AsynxFactory,
) (*Container, error) {
	c := &Container{hub: h}
	ws, err := workspace.New(adapters, func(w domain.Workspace) {
		c.broadcastWorkspace(context.Background(), w)
	}, asynxFactory)
	if err != nil {
		return nil, err
	}
	db := adapters.GlobalView()
	ch, err := chat.New(axChat, db, func(domain.Chat) {})
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
