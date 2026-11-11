package main

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// openPlacement opens the Tree the daemon keeps chat placements in, located
// by the same facts the daemon's own wiring reads.
func openPlacement(
	ctx context.Context,
	adapters *adapter.Container,
	chats asynx.Asynx[domain.Chat],
) (*repotree.Ports, error) {
	axTree, err := buildAsynx(adapters.TreeES(), adapters.TreeSS(), repotree.Schema)
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: asynx tree: %w", err)
	}
	axWorkspace, err := buildAsynx[domain.Workspace](adapters.WorkspaceES(), adapters.WorkspaceSS())
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: asynx workspace: %w", err)
	}
	projects, err := storesqlite.NewFromDB[domain.Project, string](adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: project store: %w", err)
	}
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: repository store: %w", err)
	}
	locate := repotree.NewLocator(repotree.LocatorFacts{
		ChatWorkspace: func(ctx context.Context, id string) (string, error) {
			c, err := chats.Get(ctx, id)
			return c.WorkspaceID, err
		},
		WorkspaceProject: func(ctx context.Context, id string) (string, error) {
			ws, err := axWorkspace.Get(ctx, id)
			return ws.ProjectID, err
		},
		RepoProject: func(ctx context.Context, id string) (string, error) {
			r, err := repos.FindByKey(ctx, id)
			if err != nil || r == nil {
				return "", fmt.Errorf("repo %s: %w", id, problem.ErrNotFound)
			}
			return r.ProjectID, nil
		},
	})
	all, err := projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: list projects: %w", err)
	}
	ids := make([]string, len(all))
	for i, p := range all {
		ids[i] = p.ID
	}
	ports, err := repotree.NewPorts(ctx, repotree.New(axTree), locate, ids)
	if err != nil {
		return nil, fmt.Errorf("crowbar-seed-chat: placement: %w", err)
	}
	return ports, nil
}
