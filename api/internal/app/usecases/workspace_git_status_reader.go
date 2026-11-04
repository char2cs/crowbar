package usecases

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// workspaceGitStatusReader adapts the workspace usecase into the chat tree
// usecase's WorkspaceGitStatus seam (internal/app/usecases/chat/internal/tree.
// WorkspaceGitStatus): the read-model facts about a workspace the tree places
// rows by, never a live git call.
type workspaceGitStatusReader struct {
	workspace workspace.Usecase
	// repos resolves a repo id to its project and workspaces.
	repos store.ScopedStore[domain.Repository, string]
}

// RepoOf implements agentusecase.TreeWorkspaceGitStatus.
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

// RepoIDsForHome implements agentusecase.TreeWorkspaceGitStatus: every repo
// id in home workspace homeWorkspaceID's project, so a bare-root placement in
// one project never renumbers another project's repo Node row.
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

// OutlivesItsChats implements agentusecase.TreeWorkspaceGitStatus: a
// project's home and a repo's default checkout are never reaped with a chat.
func (w workspaceGitStatusReader) OutlivesItsChats(ctx context.Context, workspaceID string) (bool, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return ws.Kind == domain.WorkspaceKindHome || ws.IsDefault, nil
}

// RendersAsBranch implements agentusecase.TreeWorkspaceGitStatus.
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

// HomeOfRepo implements agentusecase.TreeWorkspaceGitStatus.
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

// CreatedAtOf implements agentusecase.TreeWorkspaceGitStatus.
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

// BranchRowsOf implements agentusecase.TreeWorkspaceGitStatus.
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

// Exists implements agentusecase.TreeWorkspaceGitStatus.
func (w workspaceGitStatusReader) Exists(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	_, err := w.workspace.Get(ctx, workspaceID)
	return err == nil, nil
}

// Provisioned implements agentusecase.TreeWorkspaceGitStatus. An id that does
// not resolve answers true — see the port's own doc: a failed read is not
// evidence of a placeholder, and this guard only ever refuses.
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

// DefaultWorkspaceOf implements agentusecase.TreeRepoRoots: repoID's default
// checkout, "" when the repo has none.
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

// VisibleForkParent implements agentusecase.TreeWorkspaceGitStatus. The
// reduction to "" lives here, over the aggregate that carries the two fields
// it reads, rather than in the tree package: a fork parent that draws no row
// of its own is the repo's DEFAULT checkout (which IS that tree's root — see
// the port's own doc) or a project HOME workspace, and both are answered off
// domain.Workspace directly. A parent id that no longer resolves is reduced
// the same way, for the same reason: there is no space left to hold a row to.
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
