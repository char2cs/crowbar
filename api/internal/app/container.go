package app

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the application layer: the hub, the aggregate repositories, and
// the GORM CRUD stores.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
}

// New constructs the application layer from the engine and adapter containers,
// wires hub projections, and runs AgentRun crash recovery synchronously before
// returning (00 §6.2, §7).
func New(
	ctx context.Context,
	_ *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axWorkspace, err := newAsynx[domain.Workspace](adapters.WorkspaceES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	axChat, err := newAsynx[domain.Chat](adapters.ChatES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx chat: %w", err)
	}
	axAgentRun, err := newAsynx[domain.AgentRun](adapters.AgentRunES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent run: %w", err)
	}
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES)
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}

	gormStores, err := newGORMStores(adapters.DB)
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	repos, err := repositories.New(adapters.DB, h, axWorkspace, axChat, axAgentRun, axReviewThread)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}
	if err := repos.RegisterHubProjections(axAgentRun); err != nil {
		return nil, fmt.Errorf("app: hub projections: %w", err)
	}
	repos.RecoverOrphans(ctx)

	return &Container{
		Hub:          h,
		Repositories: repos,
		GORM:         gormStores,
	}, nil
}
