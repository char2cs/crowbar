package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/blame"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/branches"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	gitlog "github.com/char2cs/crowbar/api/internal/engine/git/internal/log"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/stash"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/status"
)

type execFn func(ctx context.Context, dir string, args ...string) (gitexec.Result, error)
type execStdinFn func(ctx context.Context, dir string, stdin string, args ...string) (gitexec.Result, error)

type engine struct {
	exec      execFn
	execStdin execStdinFn
}

// New returns a new Engine that shells out to the system git binary.
func New() Engine {
	return &engine{
		exec:      gitexec.Git,
		execStdin: gitexec.GitWithStdin,
	}
}

var _ Engine = (*engine)(nil)

func (e *engine) Status(
	ctx context.Context,
	repoPath string,
) (domain.GitStatus, error) {
	return status.Parse(ctx, repoPath)
}

func (e *engine) Diff(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]domain.FileDiff, error) {
	return diff.WorkingTree(ctx, repoPath, staged)
}

func (e *engine) CommitDiff(
	ctx context.Context,
	repoPath string,
	sha string,
) (domain.MultiFileDiff, error) {
	return diff.Commit(ctx, repoPath, sha)
}

func (e *engine) Log(
	ctx context.Context,
	repoPath string,
	limit int,
	skip int,
) ([]domain.Commit, error) {
	return gitlog.List(ctx, repoPath, limit, skip)
}

func (e *engine) Blame(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]domain.BlameEntry, error) {
	return blame.File(ctx, repoPath, filePath)
}

func (e *engine) Branches(
	ctx context.Context,
	repoPath string,
) ([]domain.Branch, error) {
	return branches.List(ctx, repoPath)
}

func (e *engine) Stashes(
	ctx context.Context,
	repoPath string,
) ([]domain.Stash, error) {
	return stash.List(ctx, repoPath)
}

func (e *engine) StageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	r, err := e.exec(ctx, repoPath, "add", filePath)
	if err != nil {
		return fmt.Errorf("git: stage file: %w", err)
	}
	return gitexec.RequireSuccess("stage file", r)
}

func (e *engine) StageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	return e.applyHunk(ctx, repoPath, filePath, hunkID, false)
}

func (e *engine) UnstageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	r, err := e.exec(ctx, repoPath, "restore", "--staged", filePath)
	if err != nil {
		return fmt.Errorf("git: unstage file: %w", err)
	}
	return gitexec.RequireSuccess("unstage file", r)
}

func (e *engine) UnstageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	return e.applyHunk(ctx, repoPath, filePath, hunkID, true)
}

func (e *engine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	status, err := e.Status(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("git: discard: status: %w", err)
	}
	for _, f := range status.Files {
		if f.Path != filePath {
			continue
		}
		if f.Status == domain.GitFileStatusUntracked {
			return e.discardUntracked(ctx, repoPath, filePath)
		}
		return e.discardTracked(ctx, repoPath, filePath)
	}
	return e.discardTracked(ctx, repoPath, filePath)
}

func (e *engine) discardTracked(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	r, err := e.exec(ctx, repoPath, "restore", filePath)
	if err != nil {
		return fmt.Errorf("git: restore %s: %w", filePath, err)
	}
	return gitexec.RequireSuccess("restore", r)
}

func (e *engine) discardUntracked(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	r, err := e.exec(ctx, repoPath, "clean", "-f", filePath)
	if err != nil {
		return fmt.Errorf("git: clean %s: %w", filePath, err)
	}
	return gitexec.RequireSuccess("clean", r)
}

func (e *engine) Commit(
	ctx context.Context,
	repoPath string,
	subject string,
	body string,
) error {
	args := []string{"commit", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	r, err := e.exec(ctx, repoPath, args...)
	if err != nil {
		return fmt.Errorf("git: commit: %w", err)
	}
	return gitexec.RequireSuccess("commit", r)
}

func (e *engine) Push(
	ctx context.Context,
	repoPath string,
) error {
	r, err := e.exec(ctx, repoPath, "push")
	if err != nil {
		return fmt.Errorf("git: push: %w", err)
	}
	return gitexec.RequireSuccess("push", r)
}

func (e *engine) Fetch(
	ctx context.Context,
	repoPath string,
) error {
	r, err := e.exec(ctx, repoPath, "fetch")
	if err != nil {
		return fmt.Errorf("git: fetch: %w", err)
	}
	return gitexec.RequireSuccess("fetch", r)
}

func (e *engine) Pull(
	ctx context.Context,
	repoPath string,
	mode string,
) error {
	flag := "--no-rebase"
	if mode == "rebase" {
		flag = "--rebase"
	}
	r, err := e.exec(ctx, repoPath, "pull", flag)
	if err != nil {
		return fmt.Errorf("git: pull: %w", err)
	}
	return gitexec.RequireSuccess("pull", r)
}

func (e *engine) CreateBranch(
	ctx context.Context,
	repoPath string,
	name string,
	source string,
	switchTo bool,
) error {
	return branches.Create(ctx, repoPath, name, source, switchTo)
}

func (e *engine) RenameBranch(
	ctx context.Context,
	repoPath string,
	oldName string,
	newName string,
) error {
	return branches.Rename(ctx, repoPath, oldName, newName)
}

func (e *engine) DeleteBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	return branches.Delete(ctx, repoPath, name)
}

func (e *engine) ForceDeleteBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	return branches.ForceDelete(ctx, repoPath, name)
}

func (e *engine) SwitchBranch(
	ctx context.Context,
	repoPath string,
	name string,
) error {
	return branches.Switch(ctx, repoPath, name)
}

func (e *engine) StashPush(
	ctx context.Context,
	repoPath string,
	message string,
) error {
	return stash.Push(ctx, repoPath, message)
}

func (e *engine) StashApply(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return stash.Apply(ctx, repoPath, id)
}

func (e *engine) StashPop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return stash.Pop(ctx, repoPath, id)
}

func (e *engine) StashDrop(
	ctx context.Context,
	repoPath string,
	id string,
) error {
	return stash.Drop(ctx, repoPath, id)
}

func (e *engine) Reset(
	ctx context.Context,
	repoPath string,
	mode string,
	commit string,
) error {
	flag := "--" + mode
	r, err := e.exec(ctx, repoPath, "reset", flag, commit)
	if err != nil {
		return fmt.Errorf("git: reset: %w", err)
	}
	return gitexec.RequireSuccess("reset", r)
}

func (e *engine) Merge(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	r, err := e.exec(ctx, repoPath, "merge", branch)
	if err != nil {
		return fmt.Errorf("git: merge: %w", err)
	}
	return gitexec.RequireSuccess("merge", r)
}

func (e *engine) Rebase(
	ctx context.Context,
	repoPath string,
	onto string,
) error {
	r, err := e.exec(ctx, repoPath, "rebase", onto)
	if err != nil {
		return fmt.Errorf("git: rebase: %w", err)
	}
	return gitexec.RequireSuccess("rebase", r)
}

func (e *engine) ConflictedFiles(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	return conflicts.ConflictedFiles(ctx, repoPath)
}

func (e *engine) ConflictHunks(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]domain.ConflictHunk, error) {
	return conflicts.ParseFile(ctx, repoPath, filePath)
}

func (e *engine) ResolveHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	resolution domain.ConflictResolution,
	resolvedContent string,
) error {
	return conflicts.ResolveHunk(ctx, repoPath, filePath, hunkID, resolution, resolvedContent)
}

func (e *engine) OperationContinue(
	ctx context.Context,
	repoPath string,
) error {
	return e.operationContinue(ctx, repoPath)
}

func (e *engine) OperationAbort(
	ctx context.Context,
	repoPath string,
) error {
	return e.operationAbort(ctx, repoPath)
}

func (e *engine) WorktreeAdd(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
) error {
	r, err := e.exec(ctx, repoPath, "worktree", "add", worktreePath, branch)
	if err != nil {
		return fmt.Errorf("git: worktree add: %w", err)
	}
	return gitexec.RequireSuccess("worktree add", r)
}

func (e *engine) WorktreeRemove(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	r, err := e.exec(ctx, repoPath, "worktree", "remove", "--force", worktreePath)
	if err != nil {
		return fmt.Errorf("git: worktree remove: %w", err)
	}
	return gitexec.RequireSuccess("worktree remove", r)
}

func (e *engine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]WorktreeEntry, error) {
	r, err := e.exec(ctx, repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git: worktree list: %w", err)
	}
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
	r, err := e.exec(ctx, repoPath, "rebase", "--onto", newTip, forkPoint, branch)
	if err != nil {
		return fmt.Errorf("git: rebase --onto: %w", err)
	}
	return gitexec.RequireSuccess("rebase --onto", r)
}

func (e *engine) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	r, err := e.exec(ctx, repoPath, "merge", "--ff-only", branch)
	if err != nil {
		return fmt.Errorf("git: merge --ff-only: %w", err)
	}
	return gitexec.RequireSuccess("merge --ff-only", r)
}

func (e *engine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	return e.computeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}

// ComputeStatus implements watch.GitStatusProvider.
func (e *engine) ComputeStatus(
	ctx context.Context,
	repoPath string,
) (domain.GitStatus, error) {
	return e.Status(ctx, repoPath)
}

// ComputeWorkingTreeSummary implements watch.GitStatusProvider.
func (e *engine) ComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	return e.WorkingTreeSummary(ctx, repoPath, forkPointSha)
}

func (e *engine) computeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (added int, deleted int, hasConflicts bool, hasCommits bool, err error) {
	hasConflicts, err = conflicts.HasConflicts(ctx, repoPath)
	if err != nil {
		return 0, 0, false, false, fmt.Errorf("git: summary: conflicts: %w", err)
	}

	if forkPointSha != "" {
		added, deleted, err = e.numstatFromForkPoint(ctx, repoPath, forkPointSha)
		if err != nil {
			return 0, 0, hasConflicts, false, fmt.Errorf("git: summary: numstat: %w", err)
		}
	}

	hasCommits, err = e.revListHasCommits(ctx, repoPath, forkPointSha)
	if err != nil {
		return added, deleted, hasConflicts, false, fmt.Errorf("git: summary: revlist: %w", err)
	}

	return added, deleted, hasConflicts, hasCommits, nil
}

func (e *engine) numstatFromForkPoint(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, error) {
	r, err := e.exec(ctx, repoPath, "diff", "--numstat", forkPointSha)
	if err != nil {
		return 0, 0, err
	}
	if r.ExitCode != 0 {
		return 0, 0, nil
	}
	return parseNumstat(r.Stdout)
}

func parseNumstat(
	output string,
) (int, int, error) {
	var totalAdded, totalDeleted int
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		a, _ := strconv.Atoi(fields[0])
		d, _ := strconv.Atoi(fields[1])
		if a < 0 {
			a = 0
		}
		if d < 0 {
			d = 0
		}
		totalAdded += a
		totalDeleted += d
	}
	return totalAdded, totalDeleted, nil
}

func (e *engine) revListHasCommits(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (bool, error) {
	ref := forkPointSha + "..HEAD"
	if forkPointSha == "" {
		ref = "HEAD"
	}
	r, err := e.exec(ctx, repoPath, "rev-list", "--count", ref)
	if err != nil {
		return false, err
	}
	if r.ExitCode != 0 {
		return false, nil
	}
	count, _ := strconv.Atoi(strings.TrimSpace(r.Stdout))
	return count > 0, nil
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

func detectInProgressOp(
	repoPath string,
) string {
	gitDir := filepath.Join(repoPath, ".git")
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

func (e *engine) applyHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	reverse bool,
) error {
	patch, err := diff.HunkPatch(ctx, repoPath, filePath, hunkID, reverse)
	if err != nil {
		return err
	}
	args := []string{"apply", "--cached"}
	if reverse {
		args = append(args, "-R")
	}
	r, err := e.execStdin(ctx, repoPath, patch, args...)
	if err != nil {
		return fmt.Errorf("git: apply hunk: %w", err)
	}
	return gitexec.RequireSuccess("apply hunk", r)
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
		}
	}
	if current.Path != "" {
		entries = append(entries, current)
	}
	return entries
}
