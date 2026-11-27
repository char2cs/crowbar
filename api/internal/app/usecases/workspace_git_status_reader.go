package usecases

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type workspaceGitStatusReader struct {
	workspace workspace.Usecase
	repos     store.ScopedStore[domain.Repository, string]
}

func (w workspaceGitStatusReader) RepoOf(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return ws.RepoID, nil
}

func (w workspaceGitStatusReader) RepoIDsForHome(
	ctx context.Context,
	homeWorkspaceID string,
) (map[string]bool, error) {
	ws, err := w.workspace.Get(ctx, homeWorkspaceID)
	if err != nil {
		return nil, err
	}
	rows, err := w.repos.FindWhere(ctx, domain.Repository{ProjectID: ws.ProjectID})
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}
	return ids, nil
}

func (w workspaceGitStatusReader) OutlivesItsChats(ctx context.Context, workspaceID string) (bool, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return ws.Kind == domain.WorkspaceKindHome || ws.IsDefault, nil
}

func (w workspaceGitStatusReader) RendersAsBranch(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return ws.RendersAsBranch(), nil
}

func (w workspaceGitStatusReader) HomeOfRepo(
	ctx context.Context,
	repoID string,
) (string, error) {
	repo, err := w.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		return "", err
	}
	rows, err := w.workspace.List(ctx)
	if err != nil {
		return "", err
	}
	for _, ws := range rows {
		if ws.Kind == domain.WorkspaceKindHome && ws.ProjectID == repo.ProjectID &&
			ws.Status != domain.WorkspaceStatusDeleted {
			return ws.ID, nil
		}
	}
	return "", nil
}

func (w workspaceGitStatusReader) CreatedAtOf(
	ctx context.Context,
	workspaceID string,
) (time.Time, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return time.Time{}, err
	}
	return ws.CreatedAt, nil
}

func (w workspaceGitStatusReader) BranchRowsOf(
	ctx context.Context,
	repoID string,
) ([]string, error) {
	repo, err := w.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		return nil, err
	}
	rows, err := w.workspace.ListInRepo(ctx, repo.ProjectID, repoID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, ws := range rows {
		if ws.RendersAsBranch() && !ws.IsDefault {
			ids = append(ids, ws.ID)
		}
	}
	return ids, nil
}

func (w workspaceGitStatusReader) Exists(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	_, err := w.workspace.Get(ctx, workspaceID)
	return err == nil, nil
}

func (w workspaceGitStatusReader) Provisioned(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return true, nil //nolint:nilerr // an unresolved id is not evidence of a placeholder
	}
	return ws.Provisioning.HasWorktree(), nil
}

func (w workspaceGitStatusReader) DefaultWorkspaceOf(
	ctx context.Context,
	repoID string,
) (string, error) {
	repo, err := w.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		return "", err
	}
	rows, err := w.workspace.ListInRepo(ctx, repo.ProjectID, repoID)
	if err != nil {
		return "", err
	}
	for _, ws := range rows {
		if ws.IsDefault {
			return ws.ID, nil
		}
	}
	return "", nil
}

func (w workspaceGitStatusReader) VisibleForkParent(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if ws.ParentID == "" {
		return "", nil
	}
	parent, err := w.workspace.Get(ctx, ws.ParentID)
	if err == nil && !parent.IsDefault && parent.Kind != domain.WorkspaceKindHome {
		return ws.ParentID, nil
	}
	return "", nil
}
