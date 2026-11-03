package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// MergeSquash runs `git merge --squash branch` followed by
// `git commit -m subject` inside the repo lock (07 §3.1). It classifies
// ErrConflict if the squash merge produces conflicts. A no-op squash (the child
// has no net change over the parent) stages nothing, so the commit step reports
// ErrNothingToCommit; that case is treated as success.
func (e *engine) MergeSquash(ctx context.Context, repoPath, branch, subject string) error {
	defer e.lockRepo(ctx, repoPath)()
	// The user's staged work would be committed into the squash, or dropped by
	// undoing a refused commit, so a parent holding any is refused up front.
	staged := e.exec(ctx, repoPath, "diff", "--cached", "--quiet")
	if staged.ExitCode > 1 {
		return gitexec.RequireSuccess("diff --cached", staged)
	}
	if staged.ExitCode == 1 || fileExists(filepath.Join(resolveGitDir(ctx, repoPath), "SQUASH_MSG")) {
		return fmt.Errorf("%w: merge --squash: the parent has staged changes or a squash pending", ErrDirtyTree)
	}
	if err := e.stoppable(ctx, repoPath, "merge --squash", "merge", "--squash", "--", branch); err != nil {
		return err
	}
	err := classifyGitError("merge squash commit", e.exec(ctx, repoPath, "commit", "-m", subject))
	if err == nil {
		return nil
	}
	// The index matched HEAD before the squash, so this drops only what it staged
	// and the SQUASH_MSG a no-op squash would otherwise leave pending.
	r := e.exec(context.WithoutCancel(ctx), repoPath, "reset", "--merge")
	if resetErr := gitexec.RequireSuccess("reset --merge", r); resetErr != nil {
		return fmt.Errorf("%w; restoring the worktree: %w", err, resetErr)
	}
	if errors.Is(err, ErrNothingToCommit) {
		return nil
	}
	return err
}
