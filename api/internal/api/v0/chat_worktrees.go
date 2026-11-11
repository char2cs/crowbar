package v0

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// chatWorktrees adapts the app container to the chat handlers' Worktrees port. The
// reads come from two owners: the row and merge overlay from the workspace usecase,
// the sibling list (with its derived Working overlay) from the repository container.
type chatWorktrees struct {
	app *app.Container
}

// Get implements chathandlers.Worktrees.
func (w chatWorktrees) Get(
	ctx context.Context,
	workspaceID string,
) (domain.Workspace, error) {
	return w.app.Usecases.Workspace.Get(ctx, workspaceID)
}

// ListInRepo implements chathandlers.Worktrees.
func (w chatWorktrees) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	return w.app.Repositories.ListWorkspacesInRepo(ctx, projectID, repoID)
}

// MergeEligibilitiesFor implements chathandlers.Worktrees.
func (w chatWorktrees) MergeEligibilitiesFor(
	ctx context.Context,
	wss []domain.Workspace,
	siblings []domain.Workspace,
) []workspace.MergeEligibility {
	return w.app.Usecases.Workspace.MergeEligibilitiesFor(ctx, wss, siblings)
}
