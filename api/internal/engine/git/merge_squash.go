package git

import (
	"context"
	"errors"
)

// MergeSquash runs `git merge --squash branch` followed by
// `git commit -m subject` inside the repo lock (07 §3.1). It classifies
// ErrConflict if the squash merge produces conflicts. A no-op squash (the child
// has no net change over the parent) stages nothing, so the commit step reports
// ErrNothingToCommit; that case is treated as success.
func (e *engine) MergeSquash(
	ctx context.Context,
	repoPath string,
	branch string,
	subject string,
) error {
	unlock := e.lockRepo(repoPath)
	defer unlock()
	sq := e.exec(ctx, repoPath, "merge", "--squash", "--", branch)
	if err := classifyGitError("merge --squash", sq); err != nil {
		return err
	}
	c := e.exec(ctx, repoPath, "commit", "-m", subject)
	err := classifyGitError("merge squash commit", c)
	if errors.Is(err, ErrNothingToCommit) {
		return nil
	}
	return err
}
