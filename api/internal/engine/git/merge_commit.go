package git

import "context"

func (e *engine) MergeCommit(ctx context.Context, repoPath, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "merge --no-ff", "merge", "--no-ff", "--no-edit", "--", branch)
}
