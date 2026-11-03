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
	// diff-index, unlike `diff --cached`, sees intent-to-add entries.
	staged := e.exec(ctx, repoPath, "diff-index", "--cached", "--quiet", "HEAD")
	if staged.ExitCode > 1 {
		return gitexec.RequireSuccess("diff-index --cached", staged)
	}
	if staged.ExitCode == 1 {
		return parentRefusal("the parent worktree has staged changes: commit or unstage them, then merge again")
	}
	if fileExists(filepath.Join(resolveGitDir(ctx, repoPath), "SQUASH_MSG")) {
		return parentRefusal("the parent worktree has a squash merge pending: commit or reset it, then merge again")
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

// parentRefusal is ErrDirtyTree told in plain words, for the user to act on.
type parentRefusal string

func (r parentRefusal) Error() string { return string(r) }

func (parentRefusal) Unwrap() error { return ErrDirtyTree }
