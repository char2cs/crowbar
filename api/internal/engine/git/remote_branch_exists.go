package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// RemoteBranchExists reports whether branch exists on the `origin` remote. It
// runs `git ls-remote --heads origin <branch>`, which queries the remote live
// (not stale remote-tracking refs); non-empty output means the head exists.
func (e *engine) RemoteBranchExists(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	r := e.exec(ctx, repoPath, "ls-remote", "--heads", "origin", branch)
	if err := gitexec.RequireSuccess("remote branch exists", r); err != nil {
		return false, err
	}
	return strings.TrimSpace(r.Stdout) != "", nil
}
