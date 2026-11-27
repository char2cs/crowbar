package project

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/provision"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *projectImport) provisionProtectedWorktrees(
	ctx context.Context,
	repo domain.Repository,
	protected []string,
	progress RepoProgress,
) {
	if len(protected) == 0 {
		return
	}
	home, homeErr := u.deps.CrowbarHome()
	if homeErr == nil && home == "" {
		homeErr = errNoCrowbarHome
	}
	for _, branch := range protected {
		has, err := u.repoHasBranch(ctx, repo.Path, branch)
		if err == nil && !has {
			continue
		}
		wsID := uuid.NewString()
		if err == nil && homeErr != nil {
			err = fmt.Errorf("crowbar home: %w", homeErr)
		}
		if err == nil {
			err = u.provisionProtectedBranchWorktree(ctx, repo, branch, wsID, home)
		}
		progress.Workspace(wsID, branch, err)
	}
}

func (u *projectImport) repoHasBranch(ctx context.Context, repoPath, branch string) (bool, error) {
	local, err := u.deps.Git.LocalBranchExists(ctx, repoPath, branch)
	if err != nil {
		return false, fmt.Errorf("look up branch %q: %w", branch, err)
	}
	if local {
		return true, nil
	}
	remote, err := u.deps.Git.RemoteTrackingBranchExists(ctx, repoPath, branch)
	if err != nil {
		return false, fmt.Errorf("look up origin/%s: %w", branch, err)
	}
	return remote, nil
}

func (u *projectImport) provisionProtectedBranchWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	wsID string,
	crowbarHome string,
) error {
	outcome, err := holder.Resolve(ctx, u.deps.Git, repo.Path, branch, crowbarHome)
	if err != nil {
		return fmt.Errorf("resolve holder for %q: %w", branch, err)
	}
	if outcome.Kind != holder.Free {
		return u.createPlaceholderWorkspace(ctx, repo, branch, wsID, outcome.HeldByPath)
	}
	slug := worktreepath.RemoteSlug(repo)
	siblings, err := worktreepath.SiblingRoots(crowbarHome, repo.ProjectID, slug)
	if err != nil {
		return fmt.Errorf("scan sibling worktrees for %q: %w", branch, err)
	}
	pathBranch, err := worktreepath.FreePathBranch(
		crowbarHome, repo.ProjectID, slug, branch, siblings)
	if err != nil {
		return fmt.Errorf("worktree path for %q: %w", branch, err)
	}
	path, err := worktreepath.Derive(crowbarHome, repo.ProjectID, slug, pathBranch)
	if err != nil {
		return fmt.Errorf("derive worktree path for %q: %w", branch, err)
	}
	startSha, err := u.addProtectedWorktree(ctx, repo, branch, path)
	if err != nil {
		return err
	}
	in := workspace.CreateInput{
		ID:           wsID,
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		Protected:    true,
		Provisioning: domain.WorkspaceProvisioned,
	}
	if _, err := u.createOwnedWorkspace(ctx, in, u.deps.Now()); err != nil {
		if rmErr := u.deps.Git.WorktreeRemove(ctx, repo.Path, path, true); rmErr != nil {
			slog.WarnContext(ctx, "project import: failed to clean up orphaned worktree",
				"path", path, "error", rmErr)
		}
		return fmt.Errorf("create protected workspace row: %w", err)
	}
	return nil
}

func (u *projectImport) createPlaceholderWorkspace(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	wsID string,
	heldByPath string,
) error {
	in := workspace.CreateInput{
		ID:           wsID,
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       branch,
		Protected:    true,
		HeldByPath:   heldByPath,
		Provisioning: domain.WorkspacePlaceholder,
	}
	if _, err := u.createOwnedWorkspace(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("create placeholder workspace for %q: %w", branch, err)
	}
	return nil
}

func (u *projectImport) addProtectedWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	path string,
) (string, error) {
	sha, err := provision.ExistingBranch(ctx, u.deps.Git, repo.Path, branch, path)
	if err != nil {
		return "", fmt.Errorf("add worktree for protected branch %q: %w", branch, err)
	}
	return sha, nil
}
