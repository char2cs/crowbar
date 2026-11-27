package git

import (
	"context"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) RemoteTrackingBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return e.refExists(ctx, repoPath, "refs/remotes/origin/"+branch, "remote tracking branch exists")
}

func (e *engine) LocalBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	return e.refExists(ctx, repoPath, "refs/heads/"+branch, "local branch exists")
}

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
