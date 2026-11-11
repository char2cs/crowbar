package git

import "context"

// MergeCommit is Merge that never fast-forwards: the parent gains a merge
// commit even when the branch is simply ahead of it.
func (e *engine) MergeCommit(ctx context.Context, repoPath, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "merge --no-ff", "merge", "--no-ff", "--no-edit", "--", branch)
}
