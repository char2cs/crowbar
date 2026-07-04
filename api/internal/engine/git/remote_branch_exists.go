package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// RemoteBranchExists reports whether branch exists on the `origin` remote. It
// runs `git ls-remote --heads origin <branch>`, which queries the remote live
// (not stale remote-tracking refs); non-empty output means the head exists.
//
// A failure to reach `origin` — a local-only repo with no remote, an offline
// machine, or a missing/unauthenticated remote — is treated as "the branch is
// not on any usable remote" (returns false, no error) so the caller falls back
// to creating the branch locally. Only a reachable remote that genuinely lacks
// the branch and one that has it are distinguished by the command's output.
//
// The query runs under netQueryTimeout: a remote whose connection has gone
// dead degrades to the same "not on any usable remote" answer instead of
// stalling the caller (workspace create / project import) for the OS TCP
// retransmission timeout.
func (e *engine) RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	nctx, cancel := context.WithTimeout(ctx, netQueryTimeout)
	defer cancel()
	r := e.exec(nctx, repoPath, "ls-remote", "--heads", "origin", branch)
	if err := gitexec.RequireSuccess("remote branch exists", r); err != nil {
		return false, nil
	}
	return strings.TrimSpace(r.Stdout) != "", nil
}
