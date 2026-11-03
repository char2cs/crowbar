package git

import (
	"context"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// RemoteTrackingBranchExists reports whether refs/remotes/origin/<branch> exists locally. Unlike the
// live RemoteBranchExists it cannot hiccup offline and agrees with `git branch -r`, the list the user
// picked from. Missing is (false, nil); only exit >= 2 is an error.
func (e *engine) RemoteTrackingBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	r := e.exec(ctx, repoPath, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	if r.ExitCode == 0 {
		return true, nil
	}
	if r.ExitCode == 1 {
		return false, nil
	}
	return false, gitexec.RequireSuccess("remote tracking branch exists", r)
}

// LocalBranchExists reports whether refs/heads/<branch> is present; a missing
// ref is (false, nil), the same contract as RemoteTrackingBranchExists.
func (e *engine) LocalBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	r := e.exec(ctx, repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if r.ExitCode == 0 {
		return true, nil
	}
	if r.ExitCode == 1 {
		return false, nil
	}
	return false, gitexec.RequireSuccess("local branch exists", r)
}
