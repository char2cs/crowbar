package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// RemoteBranchExists reports whether branch is on origin via a live `git ls-remote
// --heads`. An unreachable, missing or dead origin (bounded by netQueryTimeout) answers
// false with no error, so the caller falls back to creating the branch locally.
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
