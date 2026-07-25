package worktree

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameBranch renames a managed workspace's branch and relocates the workspace
// to the directory the new name derives, keeping git, the filesystem and the
// workspace record in agreement.
//
// All three have to move together. A managed worktree lives at
// <home>/projects/<project>/<slug>/<branch>/worktree, so the branch name is
// part of the path: renaming the branch alone would leave the record pointing
// at a directory named after the OLD branch and, worse, leave that directory
// squatting the name — blocking a future workspace on a branch called that.
//
// The relocation renames the whole WORKSPACE ROOT in a single os.Rename rather
// than moving the worktree leaf, because the root holds the git worktree and
// the agent `chats` tree as siblings; moving only the worktree would orphan a
// workspace's entire chat history. One rename also gives clean failure
// semantics: it either happened or it did not.
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
		// that folder and will not move it.
		return domain.Workspace{}, fmt.Errorf(
			"%w (workspace %s at %q)", ErrRenameUnmanagedWorkspace, ws.ID, ws.WorktreePath)
	}
	slug, err := u.resolveSlug(ctx, ws.RepoID, "")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rename branch: resolve worktree slug: %w", err)
	}
	slugDir := worktreepath.SlugDir(home, ws.ProjectID, slug)
	newWorktreePath, err := u.deriveRenamedWorktreePath(home, slug, ws, newBranch)
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
	// THE BRANCH RENAME HAS LANDED, so nothing after this point may be abandoned
	// because the caller went away. The endpoint is synchronous, so the request
	// context reaches every git call through exec.CommandContext: on the request
	// context a client disconnect kills the worktree repair AND the compensation
	// that exists to undo it, leaving git on the new branch with the directory
	// and the record still on the old one — a branch the record no longer names,
	// which every later merge/remove then fails to find.
	//
	// A detached, bounded context is what makes "complete or fully unwind" true
	// regardless of the caller. The bound is not a synchronisation device: it is
	// the ceiling on a wedged local git, after which the process is not going to
	// finish anyway.
	finishCtx, cancelFinish := context.WithTimeout(
		context.WithoutCancel(ctx), renameCompletionTimeout)
	defer cancelFinish()

	if err := u.relocateForRename(
		finishCtx, ws, repoPath, newBranch, newWorktreePath, slugDir,
	); err != nil {
		return domain.Workspace{}, err
	}
	renamed, err := u.workspaces.RenameBranch(finishCtx, ws.ID, newBranch, newWorktreePath)
	if err != nil {
		// The record is the LAST of the three to move and the only one that can
		// still refuse — aggregate validation, OCC exhaustion, a full command
		// queue. Returning here would leave git and the disk on the new branch
		// with the record naming the old one: the same split state, reached from
		// the other end. Put the other two back.
		u.unwindRelocateForRename(finishCtx, ws, repoPath, newBranch, newWorktreePath, slugDir)
		return domain.Workspace{}, fmt.Errorf("rename branch: record: %w", err)
	}
	return renamed, nil
}

// renameCompletionTimeout bounds the detached window in which a landed branch
// rename must either complete or unwind. Every step it covers is a local
// filesystem or local-git operation, so the bound is a wedge ceiling, never a
// budget any healthy rename comes close to spending.
const renameCompletionTimeout = 2 * time.Minute

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
		// A placeholder has no tree to move yet.
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

// deriveRenamedWorktreePath resolves where the workspace must land and proves
// the destination is free. The workspace's OWN root is excluded from the clash
// scan so a case-only rename (testing → Testing) is not rejected as colliding
// with itself.
func (u *worktreeUsecase) deriveRenamedWorktreePath(
	home string,
	slug string,
	ws domain.Workspace,
	newBranch string,
) (string, error) {
	newWorktreePath, err := worktreepath.Derive(home, ws.ProjectID, slug, newBranch)
	if err != nil {
		return "", fmt.Errorf("rename branch: %w", err)
	}
	oldRoot := worktreepath.WorkspaceRoot(ws.WorktreePath)
	newRoot := worktreepath.WorkspaceRoot(newWorktreePath)

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
	if clashErr := worktreepath.DetectClash(others, newRoot); clashErr != nil {
		return "", fmt.Errorf("%w: %v", ErrRenameTargetExists, clashErr)
	}
	// Only a SUCCESSFUL stat proves the destination is occupied. Any stat error
	// (missing, or a non-directory component on the way down) means there is
	// nothing there to protect; the MkdirAll/Rename below reports the real
	// problem and the branch rename is rolled back if it does.
	//
	// A case-only rename resolves to the same directory on a case-insensitive
	// filesystem, so "it already exists" there means "it is our own tree".
	if !strings.EqualFold(oldRoot, newRoot) {
		if _, statErr := os.Lstat(newRoot); statErr == nil {
			return "", fmt.Errorf("%w (%q)", ErrRenameTargetExists, newRoot)
		}
	}
	return newWorktreePath, nil
}

// relocateForRename moves the tree behind an ALREADY-LANDED branch rename and
// re-registers it with git. If either step fails the branch rename is undone,
// so a failure leaves git and the (still untouched) record agreeing exactly as
// they did before.
//
// ctx here is the caller's RenameBranch finishCtx — detached and bounded — so
// the compensations below can never die of the cancellation they are
// compensating for.
func (u *worktreeUsecase) relocateForRename(
	ctx context.Context,
	ws domain.Workspace,
	repoPath string,
	newBranch string,
	newWorktreePath string,
	slugDir string,
) error {
	oldRoot := worktreepath.WorkspaceRoot(ws.WorktreePath)
	newRoot := worktreepath.WorkspaceRoot(newWorktreePath)

	if err := u.moveWorkspaceRoot(oldRoot, newRoot); err != nil {
		u.undoBranchRename(ctx, repoPath, newBranch, ws.Branch)
		return err
	}
	if err := u.git.WorktreeRepair(ctx, repoPath, newWorktreePath); err != nil {
		// Put everything back: the tree returns to its old path (where git's
		// registration still points) and the branch to its old name.
		if moveBackErr := os.Rename(newRoot, oldRoot); moveBackErr != nil {
			// Both the repair and the rollback failed — the tree is at the new
			// path with a stale registration. Say so loudly; the record is left
			// untouched, so nothing downstream starts trusting the new path.
			slog.Error("rename branch: worktree repair failed and rollback failed",
				"ws", ws.ID, "from", oldRoot, "to", newRoot, "err", moveBackErr)
			return fmt.Errorf("rename branch: worktree repair: %w", err)
		}
		u.undoBranchRename(ctx, repoPath, newBranch, ws.Branch)
		return fmt.Errorf("rename branch: worktree repair: %w", err)
	}
	// Only now, once nothing can move the tree BACK: pruning the emptied parent
	// before the repair committed would delete the directory the rollback above
	// renames into.
	pruneEmptiedParents(filepath.Dir(oldRoot), slugDir)
	return nil
}

// unwindRelocateForRename undoes a relocation that fully COMPLETED, because the
// record write behind it did not. It walks the forward path backwards: the
// workspace root returns to the directory the untouched record still names, git's
// registration is repaired back onto it, the nested parent the move created at
// the abandoned destination is pruned, and the branch takes its old name again.
//
// It is not the same as the rollback inside relocateForRename, which undoes a
// relocation that never got that far. There the repair had failed, so git still
// pointed at the old path and the old parents were still standing; here both have
// already moved on — which is why this one has to recreate the pruned parents (via
// moveWorkspaceRoot's MkdirAll) and re-issue the repair.
//
// Every step is best-effort and loud: there is nothing left to return an error
// to, and the record is the one thing already known to be unchanged, so leaving
// the tree where it points is the safe end state.
//
// ctx is the caller's detached, bounded finishCtx, so a client that hung up
// cannot kill the compensation.
func (u *worktreeUsecase) unwindRelocateForRename(
	ctx context.Context,
	ws domain.Workspace,
	repoPath string,
	newBranch string,
	newWorktreePath string,
	slugDir string,
) {
	oldRoot := worktreepath.WorkspaceRoot(ws.WorktreePath)
	newRoot := worktreepath.WorkspaceRoot(newWorktreePath)

	if err := u.moveWorkspaceRoot(newRoot, oldRoot); err != nil {
		// The tree is stranded at the new path with the record on the old one.
		// Say so loudly and stop: renaming the branch back on top of that would
		// only add a third disagreement.
		slog.Error("rename branch: record write failed and the workspace could not be moved back",
			"ws", ws.ID, "from", newRoot, "to", oldRoot, "err", err)
		return
	}
	if err := u.git.WorktreeRepair(ctx, repoPath, ws.WorktreePath); err != nil {
		// The tree is back where the record points but git's registration still
		// names the abandoned path — recoverable by a later repair, and strictly
		// better than also leaving the branch renamed, so carry on.
		slog.Error("rename branch: record write failed and the worktree registration could not be restored",
			"ws", ws.ID, "path", ws.WorktreePath, "err", err)
	}
	pruneEmptiedParents(filepath.Dir(newRoot), slugDir)
	u.undoBranchRename(ctx, repoPath, newBranch, ws.Branch)
}

// pruneEmptiedParents removes the directories a workspace move emptied, walking
// up from dir and stopping at floor.
//
// A nested branch name nests its workspace root a level deeper, so renaming
// feature/x to anything else leaves <slug>/feature behind — empty, invisible,
// and squatting the name: the next workspace on a branch actually CALLED
// feature is refused by the clash scan and the destination stat for a directory
// nothing occupies.
//
// It cannot delete anything it did not empty, and it cannot climb out. os.Remove
// only ever succeeds on an EMPTY directory, so a parent a sibling workspace
// still lives under stops the walk on its own; and floor — the repo's slug
// directory, itself strictly under crowbar home — is the hard bound, checked
// with the same strict under-the-root predicate the rm guards use, so neither
// floor itself nor anything above it is ever a candidate.
func pruneEmptiedParents(
	dir string,
	floor string,
) {
	for worktreepath.UnderHome(dir, floor) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// moveWorkspaceRoot relocates the workspace root, creating the intermediate
// directories a nested branch name (feature/x) needs.
func (u *worktreeUsecase) moveWorkspaceRoot(
	oldRoot string,
	newRoot string,
) error {
	//nolint:gosec // G301: workspace roots live under the user's own ~/.crowbar home and must
	// match the 0755 the rest of the worktree layout is created with; a tighter mode here would
	// make the renamed workspace's parent inconsistent with every directory beside it.
	if err := os.MkdirAll(filepath.Dir(newRoot), 0o755); err != nil {
		return fmt.Errorf("rename branch: create destination parent: %w", err)
	}
	if err := os.Rename(oldRoot, newRoot); err != nil {
		return fmt.Errorf("rename branch: move workspace: %w", err)
	}
	return nil
}

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
