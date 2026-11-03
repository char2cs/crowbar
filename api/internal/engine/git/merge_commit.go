package git

import "context"

// MergeCommit is Merge that never fast-forwards: the parent gains a merge
// commit even when the branch is simply ahead of it.
func (e *engine) MergeCommit(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	before := detectInProgressOp(ctx, repoPath)
	r := e.exec(ctx, repoPath, "merge", "--no-ff", "--no-edit", "--", branch)
	return e.settleStoppable(ctx, repoPath, before, "merge --no-ff", r)
}
