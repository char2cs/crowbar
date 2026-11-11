package git

import "context"

// RebaseThenFFMerge rebases childWorktree onto parentBranch and fast-forwards
// parentWorktree to childBranch under one repo lock, so the parent cannot move in between.
// Errors use the standard sentinels (ErrConflict, ErrRejectedNonFastForward).
func (e *engine) RebaseThenFFMerge(ctx context.Context, childWorktree, parentBranch, parentWorktree, childBranch string) error {
	defer e.lockRepo(ctx, childWorktree)()
	if err := e.stoppable(ctx, childWorktree, "rebase", "rebase", parentBranch); err != nil {
		return err
	}
	mg := e.exec(ctx, parentWorktree, "merge", "--ff-only", "--", childBranch)
	return classifyGitError("merge --ff-only", mg)
}
