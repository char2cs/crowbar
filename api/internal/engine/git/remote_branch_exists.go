package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

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
