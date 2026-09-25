package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) UncommittedFiles(
	ctx context.Context,
	worktreePath string,
) (int, error) {
	defer e.lockRepoRead(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "status", "--porcelain", "--untracked-files=all")
	if err := gitexec.RequireSuccess("status", r); err != nil {
		return 0, err
	}
	// Non -z porcelain quotes any path with a newline, so one line is one path.
	n := 0
	for _, line := range strings.Split(r.Stdout, "\n") {
		if line != "" {
			n++
		}
	}
	return n, nil
}

func (e *engine) UnmergedCommits(
	ctx context.Context,
	dir string,
	tips []string,
	dropBranch string,
) (int, error) {
	defer e.lockRepoRead(ctx, dir)()
	args := append([]string{"rev-list", "--count", "--ignore-missing"}, tips...)
	args = append(args, "--not")
	if dropBranch != "" {
		// --exclude takes a pattern relative to refs/heads/ when it precedes --branches.
		args = append(args, "--exclude="+dropBranch)
	}
	args = append(args, "--branches", "--remotes", "--tags")
	r := e.exec(ctx, dir, args...)
	if err := gitexec.RequireSuccess("rev-list --count", r); err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(r.Stdout))
	if err != nil {
		return 0, fmt.Errorf("rev-list --count: unreadable count %q: %w", r.Stdout, err)
	}
	return n, nil
}
