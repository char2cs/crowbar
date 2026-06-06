package git

import "context"

// MergeSquash runs `git merge --squash branch` followed by
// `git commit -m subject` inside the repo lock (07 §3.1). It classifies
// ErrConflict if the squash merge produces conflicts.
func (e *engine) MergeSquash(
	ctx context.Context,
	repoPath string,
	branch string,
	subject string,
) error {
	unlock := e.lockRepo(repoPath)
	defer unlock()
	sq := e.exec(ctx, repoPath, "merge", "--squash", branch)
	if err := classifyGitError("merge --squash", sq); err != nil {
		return err
	}
	c := e.exec(ctx, repoPath, "commit", "-m", subject)
	return classifyGitError("merge squash commit", c)
}
