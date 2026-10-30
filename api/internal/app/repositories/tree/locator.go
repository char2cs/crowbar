package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Locator names the project a row belongs to, from the aggregates that own the
// fact: a chat through its workspace, a workspace and a repo directly. "" means
// none can be named yet (a chat with no workspace); kind "" tries every kind.
type Locator interface {
	ProjectOf(ctx context.Context, id string, kind tree.Kind) (string, error)
}

// LocatorFacts are the reads a Locator is built from. Each answers "" or a
// problem.NotFound error for an id it does not know.
type LocatorFacts struct {
	ChatWorkspace    func(ctx context.Context, chatID string) (string, error)
	WorkspaceProject func(ctx context.Context, workspaceID string) (string, error)
	RepoProject      func(ctx context.Context, repoID string) (string, error)
}

// NewLocator builds the Locator over f.
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

// known reads a NotFound answer as "not this", which the caller goes past.
func known(project string, err error) (string, error) {
	if err == nil {
		return project, nil
	}
	if problem.CodeOf(err) == problem.NotFound {
		return "", nil
	}
	return "", fmt.Errorf("tree: locate: %w", err)
}
