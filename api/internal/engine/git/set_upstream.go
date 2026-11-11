package git

import (
	"context"
)

// SetUpstream sets branch's upstream to origin/<branch> so branch review, PR and
// ahead/behind lookups recognise it. It writes only repo config by name, so the branch
// may be checked out elsewhere; origin/<branch> must already exist locally.
func (e *engine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "branch", "--set-upstream-to=origin/"+branch, branch)
	return classifyGitError("set upstream", r)
}
