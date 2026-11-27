package git

import (
	"context"
	"fmt"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

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
	if err := makeWorktreeParent(worktreePath); err != nil {
		return "", err
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
	if r.ExitCode == 1 && strings.TrimSpace(r.Stdout) == "" {
		return "", fmt.Errorf("rev-parse %s: %w", rev, ErrBranchNotFound)
	}
	if err := gitexec.RequireSuccess("rev-parse", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

func (e *engine) WorktreeAddAtRef(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startRef string,
) (string, error) {
	startSha, err := e.revParse(ctx, repoPath, startRef)
	if err != nil {
		return "", fmt.Errorf("worktree add at ref: resolve start: %w", err)
	}
	if err := makeWorktreeParent(worktreePath); err != nil {
		return "", err
	}
	unlock := e.lockRepo(ctx, repoPath)
	defer unlock()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-B", branch, startSha)
	if rawErr := gitexec.RequireSuccess("worktree add at ref", r); rawErr != nil && isStaleWorktreeConflict(rawErr) {
		e.pruneBeforeRetry(ctx, repoPath)
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-B", branch, startSha)
	}
	if err := classifyGitError("worktree add at ref", r); err != nil {
		return "", err
	}
	return startSha, nil
}
