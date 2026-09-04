package v0

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// chatWorktrees satisfies the chat handlers' own Worktrees port (spec §5) with
// the concrete collaborators the app container holds — law 6, the container is
// the only place that knows concrete types.
//
// It is an adapter rather than a direct injection because the three reads come
// from two different places: the row and the merge overlay from the workspace
// USECASE, the repo-scoped sibling list from the REPOSITORY container (which is
// where the derived Working overlay is applied). That is the same pair the
// retired workspace list composed its own DTOs from, reached the same way, so a
// chat's git fields and a workspace's own are resolved from one source of truth
// rather than two.
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

// MergeEligibilityFor implements chathandlers.Worktrees.
func (w chatWorktrees) MergeEligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
	siblings []domain.Workspace,
) workspace.MergeEligibility {
	return w.app.Usecases.Workspace.MergeEligibilityFor(ctx, ws, siblings)
}
