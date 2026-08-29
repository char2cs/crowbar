package project

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteProjectStore is the project persistence surface the delete usecase
// needs: resolve the project row and remove it.
type DeleteProjectStore interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Project, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteRepositoryStore is the repository persistence surface the delete
// usecase needs: list every repo row (to filter by project) and remove rows.
type DeleteRepositoryStore interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteWorkspaceRepo is the workspace persistence surface the delete usecase
// needs: list every workspace row (to filter by project) and remove rows.
type DeleteWorkspaceRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteGitEngine is the git surface the delete usecase consumes to tear down
// crowbar-created worktrees: remove the worktree directory and force-delete its
// branch, always running against the repository's main path.
type DeleteGitEngine interface {
	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
	) error
	ForceDeleteBranch(
		ctx context.Context,
		repoPath string,
		name string,
	) error
}

// DeleteDeps wires the delete usecase's collaborators.
type DeleteDeps struct {
	Projects    DeleteProjectStore
	Repos       DeleteRepositoryStore
	Workspaces  DeleteWorkspaceRepo
	Git         DeleteGitEngine
	CrowbarHome func() (string, error)
	// RemoveAll deletes the entity-scoped project directory tree (worktrees,
	// storages, icon) under ~/.crowbar/projects/<P>. Defaults to os.RemoveAll
	// when nil; tests stub it to assert the exact path removed.
	RemoveAll func(path string) error
}

// DeleteUsecase removes a project and cascades over its records: every
// workspace row and repo row owned by the project is deleted, then the project
// row itself.
//
// Disk safety (00 §5.7 import adoption vs 07 §5 teardown): the user's real
// repository directories are NEVER touched. Only crowbar-created worktree
// directories — unlocked workspaces whose path differs from the repo root —
// are removed from disk (git worktree remove + force branch delete, mirroring
// the workspace cascade-delete teardown). Locked workspaces and adopted
// worktrees (main checkouts and any pre-existing user worktree picked up at
// import, whose WorktreePath equals repo.Path) lose only their record. Disk
// teardown is best-effort: a failed worktree removal is logged and the record
// cascade continues, because the project purge must not be blocked by a stale
// git state.
type DeleteUsecase interface {
	Delete(
		ctx context.Context,
		id string,
	) error
}

type projectDelete struct {
	deps DeleteDeps
}

// NewDelete builds a DeleteUsecase from its dependencies.
func NewDelete(
	deps DeleteDeps,
) DeleteUsecase {
	if deps.RemoveAll == nil {
		deps.RemoveAll = os.RemoveAll
	}
	return &projectDelete{deps: deps}
}

func (u *projectDelete) Delete(
	ctx context.Context,
	id string,
) error {
	p, err := u.deps.Projects.FindByKey(ctx, id)
	if err != nil {
		return fmt.Errorf("project delete: find project: %w", err)
	}
	if p == nil {
		return fmt.Errorf("project delete: id %s: %w", id, apperr.ErrNotFound)
	}
	repos, err := u.projectRepos(ctx, id)
	if err != nil {
		return err
	}
	if err := u.deleteWorkspaces(ctx, id, repos); err != nil {
		return err
	}
	if err := u.deleteRepos(ctx, repos); err != nil {
		return err
	}
	if err := u.deps.Projects.Delete(ctx, id); err != nil {
		return fmt.Errorf("project delete: delete project %s: %w", id, err)
	}
	u.removeProjectDir(ctx, id)
	return nil
}

// removeProjectDir rm -rf's the entity-scoped project directory tree
// (~/.crowbar/projects/<P> — worktrees, storages, icon) once the GORM rows are
// gone. It is guarded by the crowbarHome prefix so it can NEVER touch a user's
// real repo Path or an adopted main worktree (both live outside ~/.crowbar).
// Best-effort: a removal failure is logged, not fatal, so a stale on-disk tree
// never blocks the record cascade.
func (u *projectDelete) removeProjectDir(
	ctx context.Context,
	projectID string,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.ProjectDir(home, projectID)
	if !strings.HasPrefix(dir, home) {
		return
	}
	if err := u.deps.RemoveAll(dir); err != nil {
		slog.WarnContext(ctx, "project delete: remove project dir failed; records already gone",
			"project_id", projectID, "dir", dir, "err", err)
	}
}

func (u *projectDelete) projectRepos(
	ctx context.Context,
	projectID string,
) (map[string]domain.Repository, error) {
	all, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project delete: list repos: %w", err)
	}
	owned := make(map[string]domain.Repository)
	for _, repo := range all {
		if repo.ProjectID == projectID {
			owned[repo.ID] = repo
		}
	}
	return owned, nil
}

func (u *projectDelete) deleteWorkspaces(
	ctx context.Context,
	projectID string,
	repos map[string]domain.Repository,
) error {
	all, err := u.deps.Workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("project delete: list workspaces: %w", err)
	}
	for _, ws := range all {
		if ws.ProjectID != projectID {
			continue
		}
		if err := u.deleteOneWorkspace(ctx, ws, repos); err != nil {
			return err
		}
	}
	return nil
}

func (u *projectDelete) deleteOneWorkspace(
	ctx context.Context,
	ws domain.Workspace,
	repos map[string]domain.Repository,
) error {
	u.removeWorktreeIfCrowbarManaged(ctx, ws, repos)
	if err := u.deps.Workspaces.Delete(ctx, ws.ID); err != nil {
		return fmt.Errorf("project delete: delete workspace %s: %w", ws.ID, err)
	}
	return nil
}

// removeWorktreeIfCrowbarManaged tears the workspace's worktree directory and
// branch down when, and only when, the worktree was created by crowbar: the
// workspace is unlocked, has a non-empty path, and its path differs from the
// repository root (adopted/main worktrees have WorktreePath == repo.Path and
// must never be deleted). Failures are logged and swallowed so the record
// cascade always completes.
func (u *projectDelete) removeWorktreeIfCrowbarManaged(
	ctx context.Context,
	ws domain.Workspace,
	repos map[string]domain.Repository,
) {
	repo, ok := repos[ws.RepoID]
	if !ok {
		return
	}
	if ws.Status == domain.WorkspaceStatusLocked || ws.WorktreePath == "" {
		return
	}
	// Only remove worktrees that live under the crowbar home directory.
	// Adopted worktrees (the repo's main checkout, user-created worktrees
	// imported at project-add time) live outside ~/.crowbar and must never be
	// touched. If CrowbarHome is not configured, skip disk teardown entirely.
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	if !strings.HasPrefix(ws.WorktreePath, home+"/") {
		return
	}
	if err := u.deps.Git.WorktreeRemove(ctx, repo.Path, ws.WorktreePath); err != nil {
		slog.WarnContext(ctx, "project delete: worktree remove failed; continuing record cascade",
			"workspace_id", ws.ID, "worktree_path", ws.WorktreePath, "err", err)
		return
	}
	if err := u.deps.Git.ForceDeleteBranch(ctx, repo.Path, ws.Branch); err != nil {
		slog.WarnContext(ctx, "project delete: branch delete failed; continuing record cascade",
			"workspace_id", ws.ID, "branch", ws.Branch, "err", err)
	}
}

func (u *projectDelete) deleteRepos(
	ctx context.Context,
	repos map[string]domain.Repository,
) error {
	for id := range repos {
		if err := u.deps.Repos.Delete(ctx, id); err != nil {
			return fmt.Errorf("project delete: delete repo %s: %w", id, err)
		}
	}
	return nil
}
