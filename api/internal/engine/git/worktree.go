package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	if rawErr := gitexec.RequireSuccess("worktree add", r); rawErr != nil && isStaleWorktreeConflict(rawErr) {
		// A worktree whose directory was removed out from under git still holds
		// its branch "checked out", which blocks re-adding that branch with
		// "already used by worktree" — so importing that branch fails forever.
		// Prune dead registrations and retry once. `git worktree prune` only
		// removes worktrees whose directory is gone, so a genuine conflict with a
		// live worktree still fails on the retry (correctly).
		_ = gitexec.RequireSuccess("worktree prune",
			e.exec(ctx, repoPath, "worktree", "prune"))
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	}
	return classifyGitError("worktree add", r)
}

// isStaleWorktreeConflict reports whether a worktree-add error is the
// "branch already used / checked out by another worktree" failure that pruning
// dead worktree registrations can clear.
func isStaleWorktreeConflict(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "already used by worktree") ||
		strings.Contains(m, "is already checked out")
}

func (e *engine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "remove", "--force", worktreePath)
	return gitexec.RequireSuccess("worktree remove", r)
}

// WorktreeRepair repoints the repo's administrative files at a worktree that
// has already been relocated on disk (`git worktree repair`), fixing both the
// registration's gitdir and the worktree's own .git link.
//
// Branch rename moves a workspace by renaming its whole ROOT directory in one
// os.Rename — because the root holds the git worktree and the agent `chats`
// tree as siblings, and those must travel together or the chat history is
// orphaned at the old branch's path. `git worktree move` cannot express that:
// it relocates only the worktree leaf. So the move is done by the caller and
// git is told to catch up here.
func (e *engine) WorktreeRepair(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "repair", worktreePath)
	return gitexec.RequireSuccess("worktree repair", r)
}

// WorktreePrune runs `git worktree prune`, which removes registrations for
// worktrees whose directory is gone. It only touches dead registrations, so a
// live worktree is never disturbed.
func (e *engine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "prune")
	return gitexec.RequireSuccess("worktree prune", r)
}

func (e *engine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]WorktreeEntry, error) {
	r := e.exec(ctx, repoPath, "worktree", "list", "--porcelain")
	if err := gitexec.RequireSuccess("worktree list", r); err != nil {
		return nil, err
	}
	return parseWorktreeList(r.Stdout), nil
}

// DetachWorktree puts the worktree at worktreePath on a detached HEAD at its
// current commit, freeing whatever branch it had checked out so that branch can
// be added to a NEW worktree. Working-tree files are left untouched (this only
// moves HEAD). Used to free the repo's unmanaged main folder from a branch the
// user wants to open as a real managed worktree.
func (e *engine) DetachWorktree(
	ctx context.Context,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", "--detach")
	return gitexec.RequireSuccess("switch --detach", r)
}

// CheckoutBranch switches the worktree at worktreePath onto branch, re-attaching
// a detached HEAD. Used to roll back a failed detach+add, and to restore the
// main folder once the managed worktree that held its branch is removed. Fails
// if the branch is currently checked out in another worktree.
func (e *engine) CheckoutBranch(
	ctx context.Context,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", branch)
	return classifyGitError("switch", r)
}

func (e *engine) RebaseOnto(
	ctx context.Context,
	repoPath string,
	newTip string,
	forkPoint string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "rebase", "--onto", newTip, forkPoint, branch)
	return classifyGitError("rebase --onto", r)
}

func (e *engine) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "merge", "--ff-only", branch)
	return classifyGitError("merge --ff-only", r)
}

func parseWorktreeList(
	output string,
) []WorktreeEntry {
	var entries []WorktreeEntry
	var current WorktreeEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				entries = append(entries, current)
				current = WorktreeEntry{}
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
			continue
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			continue
		}
		if line == "prunable" || strings.HasPrefix(line, "prunable ") {
			current.Prunable = true
		}
	}
	if current.Path != "" {
		entries = append(entries, current)
	}
	return entries
}
