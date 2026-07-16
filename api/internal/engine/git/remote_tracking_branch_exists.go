package git

import (
	"context"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// RemoteTrackingBranchExists reports whether the LOCAL remote-tracking ref
// `refs/remotes/origin/<branch>` is present, via
// `git show-ref --verify --quiet refs/remotes/origin/<branch>` (exit 0 ⇒ present).
//
// Unlike RemoteBranchExists — which queries `origin` LIVE over the network and
// can spuriously answer "absent" when the remote is unreachable, unauthenticated,
// times out, or the packaged daemon has a stripped PATH — this reads only refs
// git already holds on disk. It is the SAME universe `git branch -r` enumerates,
// so its answer always agrees with the branch list the import UI shows the user.
// The worktree usecase consults it first so the checkout-existing-vs-create-new
// decision can never disagree with the list the branch was picked from: a live
// query hiccup must not silently downgrade an existing remote branch into a fresh
// fork off the default branch.
//
// A missing ref exits 1 and is reported as (false, nil); only a failure that is
// neither "present" nor a clean "absent" (a broken repo, exit ≥ 2) surfaces as
// an error.
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
