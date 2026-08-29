package hierarchy

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch renames a managed workspace's branch. The workspace does not
// move.
//
// Its directory is fixed at creation and never tracked the branch afterwards,
// so a rename is a git ref rename and one record write — nothing on disk is
// touched. That is the whole operation now. It used to relocate the workspace
// root as well, because the root was named after the branch, and everything
// that followed from that is gone with it: no os.Rename of a live worktree, no
// `git worktree repair`, no id→path re-point, no detached completion window,
// and no compensating unwind for a record that refused after the disk moved.
//
// It also removes a rename this could never express: a branch renamed INTO its
// own namespace (testing → testing/x) made the destination a child of the
// source, and the kernel refuses that with EINVAL. With nothing to move, the
// case is unremarkable.
//
// The cost is that the directory keeps its original name, so a browsed tree can
// show a folder whose branch has since been renamed. Nothing resolves a
// workspace by that name — the record carries the path — and the create path
// disambiguates a name a previous workspace has frozen.
func (u *hierarchyUsecase) RenameBranch(
	ctx context.Context,
	wsID string,
	newBranch string,
) (domain.Workspace, error) {
	newBranch = strings.TrimSpace(newBranch)
	if newBranch == "" {
		return domain.Workspace{}, fmt.Errorf(
			"rename branch: blank branch name: %w", apperr.ErrInvalidArgument)
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
	// Only the record is left, and it cannot leave the two halves disagreeing in
	// a way anything has to repair: a refusal here leaves git on the new branch
	// and the record on the old one, which the next reconcile resolves from git —
	// the branch is a name, not a location, and nothing resolves a tree from it.
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
	if ws.WorktreePath == "" {
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
		// An adopted checkout: the user's own repository, which Crowbar did not
		// create. The rename no longer MOVES anything, so the old reason for
		// refusing is gone — but it would still rewrite a branch ref inside a
		// directory the user owns, and that is theirs to do.
		return fmt.Errorf(
			"%w (workspace %s at %q)", ErrRenameUnmanagedWorkspace, ws.ID, ws.WorktreePath)
	}
	return nil
}
