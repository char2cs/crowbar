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

func (e *engine) ForkPoint(
	ctx context.Context,
	repoPath string,
	upstream string,
	branch string,
) (string, error) {
	r := e.exec(ctx, repoPath, "merge-base", "--fork-point", upstream, branch)
	// Exit 1 with nothing on stdout is git finding no fork point in the reflog.
	if r.ExitCode == 1 && strings.TrimSpace(r.Stdout) == "" {
		return "", nil
	}
	if err := gitexec.RequireSuccess("merge-base --fork-point", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}

func (e *engine) BranchCreation(
	ctx context.Context,
	repoPath string,
	branch string,
) (string, error) {
	r := e.exec(ctx, repoPath, "reflog", "show", "--format=%H", "refs/heads/"+branch, "--")
	if err := gitexec.RequireSuccess("reflog show", r); err != nil {
		return "", err
	}
	entries := strings.Fields(r.Stdout)
	if len(entries) == 0 {
		return "", nil
	}
	return entries[len(entries)-1], nil
}
