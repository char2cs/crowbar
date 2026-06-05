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
func resolveGitDir(repoPath string) string {
	r, _ := gitexec.Git(
		context.Background(),
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
	repoPath string,
) string {
	gitDir := resolveGitDir(repoPath)
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
	op := detectInProgressOp(repoPath)
	switch op {
	case "rebase":
		r, err := e.exec(ctx, repoPath, "rebase", "--continue")
		if err != nil {
			return fmt.Errorf("git: rebase --continue: %w", err)
		}
		return gitexec.RequireSuccess("rebase --continue", r)
	case "merge", "squash", "pull-merge":
		r, err := e.exec(ctx, repoPath, "commit", "--no-edit")
		if err != nil {
			return fmt.Errorf("git: commit (continue): %w", err)
		}
		return gitexec.RequireSuccess("commit --no-edit", r)
	}
	return fmt.Errorf("git: operation continue: no in-progress operation detected")
}

func (e *engine) operationAbort(
	ctx context.Context,
	repoPath string,
) error {
	op := detectInProgressOp(repoPath)
	switch op {
	case "rebase":
		r, err := e.exec(ctx, repoPath, "rebase", "--abort")
		if err != nil {
			return fmt.Errorf("git: rebase --abort: %w", err)
		}
		return gitexec.RequireSuccess("rebase --abort", r)
	case "merge", "pull-merge":
		r, err := e.exec(ctx, repoPath, "merge", "--abort")
		if err != nil {
			return fmt.Errorf("git: merge --abort: %w", err)
		}
		return gitexec.RequireSuccess("merge --abort", r)
	case "squash":
		r, err := e.exec(ctx, repoPath, "reset", "--merge")
		if err != nil {
			return fmt.Errorf("git: reset --merge (squash abort): %w", err)
		}
		return gitexec.RequireSuccess("reset --merge", r)
	}
	return fmt.Errorf("git: operation abort: no in-progress operation detected")
}
