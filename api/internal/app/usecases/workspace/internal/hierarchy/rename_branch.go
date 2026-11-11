package hierarchy

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// RenameBranch renames a managed workspace's branch: a git ref rename and one record write. The directory
// is fixed at creation, so nothing on disk moves (and testing -> testing/x is unremarkable); a browsed
// folder may keep the old name, but the record carries the path.
func (u *hierarchyUsecase) RenameBranch(
	ctx context.Context,
	wsID string,
	newBranch string,
) (domain.Workspace, error) {
	newBranch = strings.TrimSpace(newBranch)
	if newBranch == "" {
		return domain.Workspace{}, fmt.Errorf(
			"rename branch: blank branch name: %w", problem.ErrInvalidArgument)
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: get workspace: %w", err)
	}
	if ws.Branch == newBranch {
		return ws, nil
	}
	if guardErr := u.guardRenameBranch(ctx, ws, newBranch); guardErr != nil {
		return domain.Workspace{}, guardErr
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: %w", err)
	}
	if err := u.git.RenameBranch(ctx, repoPath, ws.Branch, newBranch); err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: git: %w", err)
	}
	// A refusal here leaves git on the new branch and the record on the old; the next reconcile resolves it
	// from git, and nothing locates a tree by branch name.
	renamed, err := u.workspaces.RenameBranch(ctx, ws.ID, newBranch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: record: %w", err)
	}
	return renamed, nil
}

// guardRenameBranch rejects every rename that must not reach git, before any
// side effect runs.
func (u *hierarchyUsecase) guardRenameBranch(
	ctx context.Context,
	ws domain.Workspace,
	newBranch string,
) error {
	if ws.Status == domain.WorkspaceStatusLocked {
		// Protected branches own their worktree by definition (see the workspace
		// model): renaming one would desynchronise it from the provider.
		return fmt.Errorf("%w (workspace %s)", ErrWorkspaceLocked, ws.ID)
	}
	if ws.Provisioning == domain.WorkspacePlaceholder {
		// A placeholder has no branch checked out anywhere yet.
		return fmt.Errorf(
			"%w (workspace %s has no worktree yet)", ErrParentUnprovisioned, ws.ID)
	}
	exists, err := u.branchWorkspaceExists(ctx, ws.RepoID, newBranch)
	if err != nil {
		return fmt.Errorf("rename branch: %w", err)
	}
	if exists {
		return fmt.Errorf(
			"%w (repo %s, branch %q)", ErrBranchWorkspaceExists, ws.RepoID, newBranch)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return fmt.Errorf("rename branch: crowbar home: %w", err)
	}
	if !worktreepath.UnderHome(ws.WorktreePath, home) {
		// An adopted checkout is the user's own repository: rewriting a branch ref there is theirs to do.
		return fmt.Errorf(
			"%w (workspace %s at %q)", ErrRenameUnmanagedWorkspace, ws.ID, ws.WorktreePath)
	}
	return nil
}
