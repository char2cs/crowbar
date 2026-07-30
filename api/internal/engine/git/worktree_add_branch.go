package git

import (
	"context"
	"fmt"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// WorktreeAddBranch creates a new git worktree at worktreePath on a freshly
// created branch, starting at startPoint (07 §1). It resolves startPoint to a
// SHA before taking the repo lock, since lockRepo uses a non-reentrant
// sync.Mutex. Returns the start SHA so callers can record it as forkPointSha.
func (e *engine) WorktreeAddBranch(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startPoint string,
) (string, error) {
	startSha, err := e.revParse(ctx, repoPath, startPoint)
	if err != nil {
		return "", fmt.Errorf("worktree add branch: resolve start: %w", err)
	}
	unlock := e.lockRepo(ctx, repoPath)
	defer unlock()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-b", branch, startSha)
	if err := classifyGitError("worktree add branch", r); err != nil {
		return "", err
	}
	return startSha, nil
}

func (e *engine) revParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	r := e.exec(ctx, repoPath, "rev-parse", rev)
	if err := gitexec.RequireSuccess("rev-parse", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

// WorktreeAddAtRef checks branch out into a fresh worktree at worktreePath with
// the branch ref RESET to startRef (`git worktree add -B`), and returns the
// resolved start SHA.
//
// `-B` (not `-b`) is what makes an import authoritative about the remote: the
// local branch is created when absent and repointed at startRef when it already
// exists. Plain `git worktree add <path> <branch>` checks out whatever the local
// ref happens to be, which is how a diverged local copy of a branch — a leftover
// from before the repo folder was adopted, or the aftermath of a force-push —
// got imported INSTEAD of origin's commits, under a row claiming to be origin's
// branch and a fork point the checked-out history did not contain.
//
// The reset is a ref move, not a history rewrite: commits only reachable from
// the old local tip stay in the reflog, and the caller logs that tip. A branch
// checked out in another worktree still makes git refuse, which is the outcome
// the holder/placeholder path already handles.
func (e *engine) WorktreeAddAtRef(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	// Resolved before the repo lock is taken: lockRepo is a non-reentrant mutex.
	startSha, err := e.revParse(ctx, repoPath, startRef)
	if err != nil {
		return "", fmt.Errorf("worktree add at ref: resolve start: %w", err)
	}
	unlock := e.lockRepo(ctx, repoPath)
	defer unlock()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-B", branch, startSha)
	if rawErr := gitexec.RequireSuccess("worktree add at ref", r); rawErr != nil && isStaleWorktreeConflict(rawErr) {
		// Same dead-registration recovery WorktreeAdd performs: a worktree whose
		// directory vanished still holds its branch "checked out" and blocks the
		// add forever until the stale registration is pruned.
		_ = gitexec.RequireSuccess("worktree prune",
			e.exec(ctx, repoPath, "worktree", "prune"))
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-B", branch, startSha)
	}
	if err := classifyGitError("worktree add at ref", r); err != nil {
		return "", err
	}
	return startSha, nil
}
