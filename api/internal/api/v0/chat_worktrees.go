package v0

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatWorktrees struct {
	app *app.Container
}

func (w chatWorktrees) Get(
	ctx context.Context,
	workspaceID string,
) (domain.Workspace, error) {
	return w.app.Usecases.Workspace.Get(ctx, workspaceID)
}

func (w chatWorktrees) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	return w.app.Repositories.ListWorkspacesInRepo(ctx, projectID, repoID)
}

func (w chatWorktrees) MergeEligibilitiesFor(
	ctx context.Context,
	wss []domain.Workspace,
	siblings []domain.Workspace,
) []workspace.MergeEligibility {
	return w.app.Usecases.Workspace.MergeEligibilitiesFor(ctx, wss, siblings)
}
