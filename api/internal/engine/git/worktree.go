package git

import (
	"context"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	return gitexec.RequireSuccess("worktree add", r)
}

func (e *engine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "remove", "--force", worktreePath)
	return gitexec.RequireSuccess("worktree remove", r)
}

func (e *engine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]WorktreeEntry, error) {
	r := e.exec(ctx, repoPath, "worktree", "list", "--porcelain")
	if err := gitexec.RequireSuccess("worktree list", r); err != nil {
		return nil, err
	}
	return parseWorktreeList(r.Stdout), nil
}

func (e *engine) RebaseOnto(
	ctx context.Context,
	repoPath string,
	newTip string,
	forkPoint string,
	branch string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "rebase", "--onto", newTip, forkPoint, branch)
	return classifyGitError("rebase --onto", r)
}

func (e *engine) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "merge", "--ff-only", branch)
	return classifyGitError("merge --ff-only", r)
}

func parseWorktreeList(
	output string,
) []WorktreeEntry {
	var entries []WorktreeEntry
	var current WorktreeEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				entries = append(entries, current)
				current = WorktreeEntry{}
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
			continue
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			continue
		}
		if line == "prunable" || strings.HasPrefix(line, "prunable ") {
			current.Prunable = true
		}
	}
	if current.Path != "" {
		entries = append(entries, current)
	}
	return entries
}
