package git

import (
	"context"
)

func (e *engine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "branch", "--set-upstream-to=origin/"+branch, branch)
	return classifyGitError("set upstream", r)
}
