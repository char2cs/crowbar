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
	unlock := e.lockRepo(repoPath)
	defer unlock()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-b", branch, startSha)
	if err := gitexec.RequireSuccess("worktree add branch", r); err != nil {
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
