package git

import "context"

func (e *engine) RebaseThenFFMerge(ctx context.Context, childWorktree, parentBranch, parentWorktree, childBranch string) error {
	defer e.lockRepo(ctx, childWorktree)()
	if err := e.stoppable(ctx, childWorktree, "rebase", "rebase", parentBranch); err != nil {
		return err
	}
	mg := e.exec(ctx, parentWorktree, "merge", "--ff-only", "--", childBranch)
	return classifyGitError("merge --ff-only", mg)
}
