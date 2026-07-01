package git

import "context"

// RebaseThenFFMerge replays childWorktree onto parentBranch and then
// fast-forwards parentWorktree to childBranch as a single locked unit (07 §3.1),
// so nothing can advance the parent between the two steps and break the
// --ff-only. It takes the shared repo lock once and classifies errors with the
// standard sentinels (ErrConflict on a conflicting rebase, ErrRejectedNonFastForward
// if the parent moved).
func (e *engine) RebaseThenFFMerge(
	ctx context.Context,
	childWorktree string,
	parentBranch string,
	parentWorktree string,
	childBranch string,
) error {
	defer e.lockRepo(ctx, childWorktree)()
	rb := e.exec(ctx, childWorktree, "rebase", parentBranch)
	if err := classifyGitError("rebase", rb); err != nil {
		return err
	}
	mg := e.exec(ctx, parentWorktree, "merge", "--ff-only", "--", childBranch)
	return classifyGitError("merge --ff-only", mg)
}
