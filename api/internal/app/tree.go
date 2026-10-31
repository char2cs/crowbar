package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/app/migratetree"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// newPlacement opens the id-keyed placement ports over every project's Tree and
// announces each chat a Tree write moved to live clients, as the chat
// aggregate's own events did before placement moved into the Tree.
func newPlacement(
	ctx context.Context,
	ax *aggregates,
	gormStores *GORMStores,
	chatSnapshots *agentusecase.ChatSnapshots,
) (*repotree.Ports, error) {
	locate := repotree.NewLocator(repotree.LocatorFacts{
		ChatWorkspace: func(ctx context.Context, chatID string) (string, error) {
			chat, err := ax.agentChat.Get(ctx, chatID)
			return chat.WorkspaceID, err
		},
		WorkspaceProject: func(ctx context.Context, wsID string) (string, error) {
			ws, err := ax.workspace.Get(ctx, wsID)
			return ws.ProjectID, err
		},
		RepoProject: func(ctx context.Context, repoID string) (string, error) {
			repo, err := gormStores.Repositories.FindByKey(ctx, repoID)
			if err != nil || repo == nil {
				return "", fmt.Errorf("repo %s: %w", repoID, problem.ErrNotFound)
			}
			return repo.ProjectID, nil
		},
	})
	projects, err := gormStores.Projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: placement: list projects: %w", err)
	}
	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	ports, err := repotree.NewPorts(ctx, repotree.New(ax.tree), locate, ids)
	if err != nil {
		return nil, fmt.Errorf("app: placement: %w", err)
	}
	moved := func(ctx context.Context, chatID string, p tree.Placement, kind string) {
		chatSnapshots.ApplyPlacement(ctx, chatID, p.ParentID, p.Order, kind)
	}
	if err := repotree.Announce(ax.tree, moved); err != nil {
		return nil, fmt.Errorf("app: placement: %w", err)
	}
	return ports, nil
}

// legacySources are the stores the placement-tree step reads the retired
// placement records and their facts from.
func legacySources(
	adapters *adapter.Container,
	repos *repositories.Container,
	gormStores *GORMStores,
) migratetree.LegacySources {
	return migratetree.LegacySources{
		ChatLog:    adapters.AgentChatES(),
		NodeLog:    filepath.Join(adapters.Home().Events(), migrations.LegacyNodeLog),
		View:       adapters.GlobalView(),
		Projects:   gormStores.Projects.FindAll,
		Workspaces: repos.Workspace.List,
		Repos: func(ctx context.Context) ([]domain.Repository, error) {
			return gormStores.Repositories.FindAll(ctx)
		},
	}
}
