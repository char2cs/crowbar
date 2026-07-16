package git

import (
	"context"
)

// SetUpstream sets branch's upstream to origin/<branch>
// (`git branch --set-upstream-to=origin/<branch> <branch>`), linking a local
// branch back to origin so it is recognised as origin's <branch> for
// branch-review/PR/base lookups and ahead/behind reporting.
//
// It operates on the branch by NAME and writes only the shared repo config, so it
// works whether or not <branch> is the currently checked-out branch of repoPath's
// worktree — the imported-branch checkout path runs it after checking the branch
// out into a SEPARATE worktree. origin/<branch> must already exist locally as a
// remote-tracking ref (the import path guarantees this before calling).
func (e *engine) SetUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "branch", "--set-upstream-to=origin/"+branch, branch)
	return classifyGitError("set upstream", r)
}
