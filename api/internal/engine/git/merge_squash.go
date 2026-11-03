package git

import (
	"context"
	"errors"
	"fmt"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// MergeSquash runs `git merge --squash branch` followed by
// `git commit -m subject` inside the repo lock (07 §3.1). It classifies
// ErrConflict if the squash merge produces conflicts. A no-op squash (the child
// has no net change over the parent) stages nothing, so the commit step reports
// ErrNothingToCommit; that case is treated as success.
func (e *engine) MergeSquash(ctx context.Context, repoPath, branch, subject string) error {
	defer e.lockRepo(ctx, repoPath)()
	if err := e.stoppable(ctx, repoPath, "merge --squash", "merge", "--squash", "--", branch); err != nil {
		return err
	}
	err := classifyGitError("merge squash commit", e.exec(ctx, repoPath, "commit", "-m", subject))
	if err == nil || errors.Is(err, ErrNothingToCommit) {
		return nil
	}
	// A squash runs only over an index matching HEAD, so this drops only what it staged.
	r := e.exec(context.WithoutCancel(ctx), repoPath, "reset", "--merge")
	if resetErr := gitexec.RequireSuccess("reset --merge", r); resetErr != nil {
		return fmt.Errorf("%w; restoring the worktree: %w", err, resetErr)
	}
	return err
}
