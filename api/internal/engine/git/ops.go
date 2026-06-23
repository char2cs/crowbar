package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// resolveGitDir returns the real .git directory for repoPath. In a secondary
// git worktree the .git entry is a file, not a directory; we ask git itself so
// in-progress marker files (MERGE_HEAD, rebase-merge/, …) are always found.
func resolveGitDir(ctx context.Context, repoPath string) string {
	r := gitexec.Git(
		ctx,
		repoPath,
		"rev-parse",
		"--git-dir",
	)
	if r.ExitCode != 0 {
		return filepath.Join(repoPath, ".git")
	}
	dir := strings.TrimSpace(r.Stdout)
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(repoPath, dir)
}

func detectInProgressOp(
	ctx context.Context,
	repoPath string,
) string {
	gitDir := resolveGitDir(ctx, repoPath)
	if fileExists(filepath.Join(gitDir, "rebase-merge")) {
		return "rebase"
	}
	if fileExists(filepath.Join(gitDir, "rebase-apply")) {
		return "rebase"
	}
	if fileExists(filepath.Join(gitDir, "SQUASH_HEAD")) {
		return "squash"
	}
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return "merge"
	}
	return ""
}

func fileExists(
	path string,
) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (e *engine) operationContinue(
	ctx context.Context,
	repoPath string,
) error {
	op := detectInProgressOp(ctx, repoPath)
	switch op {
	case "rebase":
		r := e.exec(ctx, repoPath, "rebase", "--continue")
		return gitexec.RequireSuccess("rebase --continue", r)
	case "merge", "squash", "pull-merge":
		r := e.exec(ctx, repoPath, "commit", "--no-edit")
		return gitexec.RequireSuccess("commit --no-edit", r)
	}
	return fmt.Errorf("git: operation continue: no in-progress operation detected")
}

func (e *engine) operationAbort(
	ctx context.Context,
	repoPath string,
) error {
	op := detectInProgressOp(ctx, repoPath)
	switch op {
	case "rebase":
		r := e.exec(ctx, repoPath, "rebase", "--abort")
		return gitexec.RequireSuccess("rebase --abort", r)
	case "merge", "pull-merge":
		r := e.exec(ctx, repoPath, "merge", "--abort")
		return gitexec.RequireSuccess("merge --abort", r)
	case "squash":
		r := e.exec(ctx, repoPath, "reset", "--merge")
		return gitexec.RequireSuccess("reset --merge", r)
	}
	return fmt.Errorf("git: operation abort: no in-progress operation detected")
}
