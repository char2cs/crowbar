package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) MergeBase(
	ctx context.Context,
	repoPath string,
	a string,
	b string,
) (string, error) {
	r := e.exec(ctx, repoPath, "merge-base", a, b)
	if err := gitexec.RequireSuccess("merge-base", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}
