package git

import (
	"context"
	"fmt"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// WorktreeAddBranch adds a worktree on a new branch at startPoint, resolving it to a SHA before
// the non-reentrant repo lock; returns that SHA for forkPointSha.
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
	r := e.exec(ctx, repoPath, "rev-parse", "--verify", "--quiet", rev)
	// Under --verify --quiet absence alone exits 1 (fatal errors exit 128).
	if r.ExitCode == 1 && strings.TrimSpace(r.Stdout) == "" {
		return "", fmt.Errorf("rev-parse %s: %w", rev, ErrBranchNotFound)
	}
	if err := gitexec.RequireSuccess("rev-parse", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

// WorktreeAddAtRef checks branch out with its ref RESET to startRef (`-B`): plain add would check out
// a diverged local copy instead of origin's commits. It is a ref move (old tip stays in the reflog);
// a branch checked out elsewhere still makes git refuse, which the holder path handles.
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
		e.pruneBeforeRetry(ctx, repoPath)
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-B", branch, startSha)
	}
	if err := classifyGitError("worktree add at ref", r); err != nil {
		return "", err
	}
	return startSha, nil
}
