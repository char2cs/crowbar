package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// removeRepoDir removes the repo's entity directory (icon, storages) under the
// crowbar home. Best-effort: the row is already gone.
func (u *projectDelete) removeRepoDir(
	ctx context.Context,
	repo domain.Repository,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.RepoDir(home, repo.ProjectID, repo.ID)
	if err := removeTreeKeeping(dir, worktreepath.IsLiveCheckout); err != nil {
		slog.ErrorContext(ctx, "delete repo: remove entity dir", "repo", repo.ID, "dir", dir, "err", err)
	}
}

// deleteRemainingWorkspaces tombstones the project's rows no repo cascade took
// — its home workspace, which belongs to the project rather than to a repo.
// The delete reactor purges them like any other tombstone.
func (u *projectDelete) deleteRemainingWorkspaces(
	ctx context.Context,
	projectID string,
	all []domain.Workspace,
) error {
	for _, ws := range all {
		// A repo's rows were the repo cascade's; a row of a repo that now
		// belongs to another project is that project's, whatever its own
		// (denormalised) ProjectID still says.
		if ws.RepoID != "" || ws.ProjectID != projectID || ws.Status == domain.WorkspaceStatusDeleted {
			continue
		}
		if err := u.deps.Workspaces.Delete(ctx, ws.ID); err != nil && !alreadyGone(err) {
			return fmt.Errorf("project delete: delete workspace %s: %w", ws.ID, err)
		}
	}
	return nil
}

func (u *projectDelete) forgetNode(
	ctx context.Context,
	id string,
) {
	if u.deps.Nodes == nil {
		return
	}
	if err := u.deps.Nodes.Forget(ctx, id); err != nil && !alreadyGone(err) {
		slog.WarnContext(ctx, "project delete: forget node row", "id", id, "err", err)
	}
}

// alreadyGone reports an error meaning the thing being removed no longer
// exists — for a delete, success.
func alreadyGone(err error) bool {
	return errors.Is(err, problem.ErrNotFound) || errors.Is(err, asynxModels.ErrValidation)
}

// removeProjectDir removes ~/.crowbar/projects/<P> except a live git checkout (git refused
// to remove it, e.g. uncommitted work) and any live workspace of another project; kept
// paths keep their parents. Best-effort: the rows are gone, so failures are only logged.
func (u *projectDelete) removeProjectDir(
	ctx context.Context,
	projectID string,
	repos map[string]domain.Repository,
	all []domain.Workspace,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.ProjectDir(home, projectID)
	// Another project's live workspace keeps its whole root: the worktree and
	// the chats tree beside it.
	foreign := map[string]bool{}
	for _, ws := range all {
		if ownedBy(ws, projectID, repos) || ws.Status == domain.WorkspaceStatusDeleted || ws.WorktreePath == "" {
			continue
		}
		if root, ok := worktreepath.OwnRoot(ws.WorktreePath, home); ok {
			foreign[root] = true
			continue
		}
		// A pre-leaf row: its checkout, and the chats tree beside it.
		foreign[filepath.Clean(ws.WorktreePath)] = true
		foreign[worktreepath.ChatsDir(filepath.Clean(ws.WorktreePath))] = true
	}
	keep := func(path string) bool {
		return foreign[path] || worktreepath.IsLiveCheckout(path)
	}
	if err := removeTreeKeeping(dir, keep); err != nil {
		slog.ErrorContext(ctx, "project delete: remove project dir; records already gone, part of the directory left on disk",
			"project_id", projectID, "dir", dir, "err", err)
	}
}

// ownedBy reports whether ws is the project's: a repo's row belongs to the
// project that owns the repo (the repo row is the one owner of that
// assignment); a repo-less row — the project home — to its own ProjectID.
func ownedBy(
	ws domain.Workspace,
	projectID string,
	repos map[string]domain.Repository,
) bool {
	if ws.RepoID == "" {
		return ws.ProjectID == projectID
	}
	_, mine := repos[ws.RepoID]
	return mine
}

// removeTreeKeeping removes dir and everything under it except the paths keep
// claims. A directory is removed only once it is empty, so whatever is kept
// keeps its ancestors. It never follows a symlink.
func removeTreeKeeping(
	dir string,
	keep func(path string) bool,
) error {
	if keep(dir) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			errs = append(errs, removeTreeKeeping(path, keep))
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	// Fails (harmlessly) while something kept is still inside.
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && !isNotEmpty(dir) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func isNotEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
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
