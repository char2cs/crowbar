package app

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

// Container is the application layer: the hub, the aggregate repositories, the
// GORM CRUD stores, and the composed usecases.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
	Usecases     *usecases.Container
}

// New constructs the application layer from the engine and adapter containers,
// wires hub projections, and runs AgentRun crash recovery synchronously before
// returning (00 §6.2, §7).
func New(
	ctx context.Context,
	engines *engine.Container,
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

	ucs, err := usecases.New(repos, toUsecaseStores(gormStores), engines)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}

	startProviderSweep(ctx, engines, repos, ucs)

	return &Container{
		Hub:          h,
		Repositories: repos,
		GORM:         gormStores,
		Usecases:     ucs,
	}, nil
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:         gormStores.Projects,
		Repositories:     gormStores.Repositories,
		TerminalProfiles: gormStores.TerminalProfiles,
	}
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
			if ws.Status != domain.WorkspaceStatusPROpen {
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
