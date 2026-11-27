package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

type worktrees interface {
	WorktreeAdd(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
	) error

	WorktreeRemove(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		force bool,
	) error

	WorktreeRepair(
		ctx context.Context,
		repoPath string,
		worktreePath string,
	) error

	WorktreePrune(
		ctx context.Context,
		repoPath string,
	) error

	UncommittedFiles(
		ctx context.Context,
		worktreePath string,
	) (int, error)

	UnmergedCommits(
		ctx context.Context,
		dir string,
		tips []string,
		dropBranch string,
	) (int, error)

	WorktreeList(
		ctx context.Context,
		repoPath string,
	) ([]WorktreeEntry, error)

	DetachWorktree(
		ctx context.Context,
		worktreePath string,
	) error

	CheckoutBranch(
		ctx context.Context,
		worktreePath string,
		branch string,
	) error

	WorktreeAddBranch(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startPoint string,
	) (string, error)

	WorktreeAddAtRef(
		ctx context.Context,
		repoPath string,
		worktreePath string,
		branch string,
		startRef string,
	) (string, error)
}

type WorktreeEntry struct {
	Path     string
	Branch   string
	Head     string
	Prunable bool
}

func (e *engine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	if err := makeWorktreeParent(worktreePath); err != nil {
		return err
	}
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	if rawErr := gitexec.RequireSuccess("worktree add", r); rawErr != nil && isStaleWorktreeConflict(rawErr) {
		e.pruneBeforeRetry(ctx, repoPath)
		r = e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	}
	return classifyGitError("worktree add", r)
}

func isStaleWorktreeConflict(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "already used by worktree") ||
		strings.Contains(m, "is already checked out")
}

func (e *engine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	force bool,
) error {
	defer e.lockRepo(ctx, repoPath)()
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	r := e.exec(ctx, repoPath, append(args, worktreePath)...)
	return gitexec.RequireSuccess("worktree remove", r)
}

func (e *engine) WorktreeRepair(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "worktree", "repair", worktreePath)
	return gitexec.RequireSuccess("worktree repair", r)
}

func (e *engine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.pruneWorktrees(ctx, repoPath)
}

func (e *engine) pruneWorktrees(
	ctx context.Context,
	repoPath string,
) error {
	r := e.exec(ctx, repoPath, "worktree", "prune")
	return gitexec.RequireSuccess("worktree prune", r)
}

func (e *engine) pruneBeforeRetry(
	ctx context.Context,
	repoPath string,
) {
	if err := e.pruneWorktrees(ctx, repoPath); err != nil {
		slog.Warn("git: stale worktree registrations not pruned", "repo", repoPath, "err", err)
	}
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

func (e *engine) DetachWorktree(
	ctx context.Context,
	worktreePath string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", "--detach")
	return gitexec.RequireSuccess("switch --detach", r)
}

func (e *engine) CheckoutBranch(
	ctx context.Context,
	worktreePath string,
	branch string,
) error {
	defer e.lockRepo(ctx, worktreePath)()
	r := e.exec(ctx, worktreePath, "switch", branch)
	return classifyGitError("switch", r)
}

func (e *engine) RebaseOnto(ctx context.Context, repoPath, newTip, forkPoint, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "rebase --onto", "rebase", "--onto", newTip, forkPoint, branch)
}

func (e *engine) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
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

func makeWorktreeParent(
	worktreePath string,
) error {
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o700); err != nil {
		return fmt.Errorf("worktree add: create parent of %s: %w", worktreePath, err)
	}
	if home, ok := worktreepath.HomeOf(worktreePath); ok {
		if err := worktreepath.Reach(home, worktreePath); err != nil {
			return fmt.Errorf("worktree add: %w", err)
		}
	}
	return nil
}
