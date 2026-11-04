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
	return e.refExists(ctx, repoPath, "refs/remotes/origin/"+branch, "remote tracking branch exists")
}

// LocalBranchExists reports whether refs/heads/<branch> is present; a missing
// ref is (false, nil), the same contract as RemoteTrackingBranchExists.
func (e *engine) LocalBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return e.refExists(ctx, repoPath, "refs/heads/"+branch, "local branch exists")
}

// refExists answers whether ref is on disk: show-ref exits 1 for a missing ref, higher on failure.
func (e *engine) refExists(ctx context.Context, repoPath, ref, op string) (bool, error) {
	r := e.exec(ctx, repoPath, "show-ref", "--verify", "--quiet", ref)
	switch r.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	return false, gitexec.RequireSuccess(op, r)
}
