package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Locator interface {
	ProjectOf(ctx context.Context, id string, kind tree.Kind) (string, error)
}

type LocatorFacts struct {
	ChatWorkspace    func(ctx context.Context, chatID string) (string, error)
	WorkspaceProject func(ctx context.Context, workspaceID string) (string, error)
	RepoProject      func(ctx context.Context, repoID string) (string, error)
}

func NewLocator(f LocatorFacts) Locator {
	return factLocator(f)
}

type factLocator LocatorFacts

func (l factLocator) ProjectOf(ctx context.Context, id string, kind tree.Kind) (string, error) {
	if id == "" {
		return "", nil
	}
	switch kind {
	case tree.KindWorkspace:
		return known(l.WorkspaceProject(ctx, id))
	case tree.KindRepo:
		return known(l.RepoProject(ctx, id))
	case tree.KindChat:
		return l.chatProject(ctx, id)
	case tree.KindFolder:
		return "", nil
	}
	for _, try := range []func(context.Context, string) (string, error){l.WorkspaceProject, l.RepoProject, l.chatProject} {
		p, err := known(try(ctx, id))
		if err != nil || p != "" {
			return p, err
		}
	}
	return "", nil
}

func (l factLocator) chatProject(ctx context.Context, chatID string) (string, error) {
	ws, err := known(l.ChatWorkspace(ctx, chatID))
	if err != nil || ws == "" {
		return "", err
	}
	return known(l.WorkspaceProject(ctx, ws))
}

func known(project string, err error) (string, error) {
	if err == nil {
		return project, nil
	}
	if problem.CodeOf(err) == problem.NotFound {
		return "", nil
	}
	return "", fmt.Errorf("tree: locate: %w", err)
}
