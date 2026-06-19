// Package handlers holds the gin handlers backing the provider endpoint:
// workspace provider state poll and protected-branch lookup.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
)

// ProviderEngine is the provider engine surface the handlers need.
type ProviderEngine interface {
	// PollOnView runs an immediate poll for a workspace and returns its state.
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (provider.ProviderState, error)

	// ProtectedBranches returns protected branch names for repoPath.
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
}

// WorkspaceReader is the workspace read surface the handlers need: fetch a
// single workspace by id (used by the per-workspace provider poll) and list
// every workspace (used to resolve a repo-scoped worktree path for the
// protected-branch lookup until the repo aggregate carries a path of its own,
// W11/W12).
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

// Handlers serves the provider routes from the provider engine and workspace
// reader.
type Handlers struct {
	eng      ProviderEngine
	wsReader WorkspaceReader
}

// New builds the provider Handlers from the provider engine and workspace
// reader.
func New(
	eng ProviderEngine,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		eng:      eng,
		wsReader: wsReader,
	}
}
