package worktree

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch renames a managed workspace's branch and republishes the
// navigable alias that names it, keeping git, the filesystem and the workspace
// record in agreement.
//
// The workspace itself does not move. Its root is keyed by the workspace id
// (worktreepath.WorkspaceRootByID), so a branch name change touches no live
// directory: the <slug>/<branch> path is a symlink, and renaming republishes it.
//
// That is the whole reason the layout is keyed that way. When the root was named
// after the branch, this operation had to rename a directory holding a live git
// worktree, repair git's registration onto the new path, and carry a
// compensating unwind for every step in case a later one failed — and it still
// could not express a branch renamed INTO its own namespace (testing ->
// testing/x), where the destination is a child of the source and the kernel
// refuses with EINVAL. None of those cases exist now: git renames the ref, one
// symlink is swapped, and the record records the new name against the same path
// it already had.
func (u *worktreeUsecase) RenameBranch(
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

	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: crowbar home: %w", err)
	}
	if !worktreepath.UnderHome(ws.WorktreePath, home) {
		// An adopted checkout at the user's own directory. Crowbar does not own
		// that folder and will not rename what it names.
		return domain.Workspace{}, fmt.Errorf(
			"%w (workspace %s at %q)", ErrRenameUnmanagedWorkspace, ws.ID, ws.WorktreePath)
	}
	slug, err := u.resolveSlug(ctx, ws.RepoID, "")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: resolve worktree slug: %w", err)
	}
	oldAlias, err := worktreepath.AliasDir(home, ws.ProjectID, slug, ws.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: %w", err)
	}
	newAlias, err := u.deriveRenamedAlias(home, slug, ws, newBranch)
	if err != nil {
		return domain.Workspace{}, err
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: %w", err)
	}
	if err := u.git.RenameBranch(ctx, repoPath, ws.Branch, newBranch); err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: git: %w", err)
	}

	// The identity-keyed root the alias names. The boot pass converts every
	// workspace to this layout before the daemon serves, so there is no
	// second shape to handle here.
	root := worktreepath.WorkspaceRoot(ws.WorktreePath)
	slugDir := worktreepath.SlugDir(home, ws.ProjectID, slug)
	if linkErr := worktreepath.LinkAlias(newAlias, root); linkErr != nil {
		// Nothing has been written except git's ref, so the whole operation can
		// still be taken back cleanly.
		u.undoBranchRename(ctx, repoPath, newBranch, ws.Branch)
		return domain.Workspace{}, fmt.Errorf("rename branch: %w", linkErr)
	}
	renamed, err := u.workspaces.RenameBranch(ctx, ws.ID, newBranch)
	if err != nil {
		// The record is the last to move and the only one that can still refuse.
		// Put git and the alias back rather than leave the branch renamed under a
		// record that still names the old one.
		if unlinkErr := worktreepath.UnlinkAlias(newAlias, slugDir); unlinkErr != nil {
			slog.Error("rename branch: could not withdraw the new alias",
				"ws", ws.ID, "alias", newAlias, "err", unlinkErr)
		}
		u.undoBranchRename(ctx, repoPath, newBranch, ws.Branch)
		return domain.Workspace{}, fmt.Errorf("rename branch: record: %w", err)
	}
	// Last, because it is the only step whose failure costs nothing: a stale
	// alias is a broken link in a navigable tree, not a workspace anyone can
	// lose. Withdrawing it before the record landed would have made the rollback
	// above have to put it back.
	if unlinkErr := worktreepath.UnlinkAlias(oldAlias, slugDir); unlinkErr != nil {
		slog.Warn("rename branch: left the previous alias in place",
			"ws", ws.ID, "alias", oldAlias, "err", unlinkErr)
	}
	// Point git's registration at the REAL root rather than at an alias that just
	// moved. A workspace converted by the boot pass may still have git recording
	// the old name-derived path, which resolved only because the alias sat there
	// — and the alias no longer does. Once repaired it names the identity root,
	// so every later rename is free.
	if repairErr := u.git.WorktreeRepair(ctx, repoPath, ws.WorktreePath); repairErr != nil {
		slog.Warn("rename branch: could not repair the worktree registration",
			"ws", ws.ID, "path", ws.WorktreePath, "err", repairErr)
	}
	return renamed, nil
}

// guardRenameBranch rejects every rename that must not reach git, before any
// side effect runs.
func (u *worktreeUsecase) guardRenameBranch(
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
		// A placeholder has no tree to name yet.
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
	return nil
}

// deriveRenamedAlias resolves the alias the new name claims and proves it is
// free. The workspace's OWN alias is excluded from the clash scan so a case-only
// rename (testing -> Testing) is not rejected as colliding with itself.
func (u *worktreeUsecase) deriveRenamedAlias(
	home string,
	slug string,
	ws domain.Workspace,
	newBranch string,
) (string, error) {
	newAlias, err := worktreepath.AliasDir(home, ws.ProjectID, slug, newBranch)
	if err != nil {
		return "", fmt.Errorf("rename branch: %w", err)
	}
	oldRoot := worktreepath.WorkspaceRoot(ws.WorktreePath)

	siblings, err := siblingWorktreePaths(home, ws.ProjectID, slug)
	if err != nil {
		return "", fmt.Errorf("rename branch: scan sibling worktrees: %w", err)
	}
	others := make([]string, 0, len(siblings))
	for _, s := range siblings {
		if !strings.EqualFold(s, oldRoot) {
			others = append(others, s)
		}
	}
	if clashErr := worktreepath.DetectClash(others, newAlias); clashErr != nil {
		return "", fmt.Errorf("%w: %v", ErrRenameTargetExists, clashErr)
	}
	return newAlias, nil
}

// undoBranchRename returns git to the name it had, best effort.
func (u *worktreeUsecase) undoBranchRename(
	ctx context.Context,
	repoPath string,
	from string,
	to string,
) {
	if err := u.git.RenameBranch(ctx, repoPath, from, to); err != nil {
		slog.Error("rename branch: could not restore original branch name",
			"repo", repoPath, "from", from, "to", to, "err", err)
	}
}
