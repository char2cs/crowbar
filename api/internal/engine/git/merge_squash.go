package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) MergeSquash(ctx context.Context, repoPath, branch, subject string) error {
	defer e.lockRepo(ctx, repoPath)()
	if err := e.refuseUnsettledParent(ctx, repoPath); err != nil {
		return err
	}
	if err := e.stoppable(ctx, repoPath, "merge --squash", "merge", "--squash", "--", branch); err != nil {
		return err
	}
	err := classifyGitError("merge squash commit", e.exec(ctx, repoPath, "commit", "-m", subject))
	if err == nil {
		return nil
	}
	r := e.exec(context.WithoutCancel(ctx), repoPath, "reset", "--merge")
	if resetErr := gitexec.RequireSuccess("reset --merge", r); resetErr != nil {
		return fmt.Errorf("%w; restoring the worktree: %w", err, resetErr)
	}
	if errors.Is(err, ErrNothingToCommit) {
		return nil
	}
	return err
}

func (e *engine) refuseUnsettledParent(ctx context.Context, repoPath string) error {
	staged := e.exec(ctx, repoPath, "diff-index", "--cached", "--quiet", "HEAD")
	if staged.ExitCode > 1 {
		if e.exec(ctx, repoPath, "rev-parse", "--verify", "--quiet", "HEAD").ExitCode != 0 {
			return parentRefusal("the parent branch has no commits yet: commit to it first, then merge again")
		}
		return gitexec.RequireSuccess("diff-index --cached", staged)
	}
	if staged.ExitCode == 1 {
		return parentRefusal("the parent worktree has staged changes: commit or unstage them, then merge again")
	}
	if fileExists(filepath.Join(resolveGitDir(ctx, repoPath), "SQUASH_MSG")) {
		return parentRefusal("the parent worktree has a squash merge pending: commit or reset it, then merge again")
	}
	return nil
}

type parentRefusal string

func (r parentRefusal) Error() string { return string(r) }

func (parentRefusal) Unwrap() error { return ErrDirtyTree }
