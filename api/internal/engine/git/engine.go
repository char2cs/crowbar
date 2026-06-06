package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/blame"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/branches"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	gitlog "github.com/char2cs/crowbar/api/internal/engine/git/internal/log"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/stash"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/status"
)

type (
	execFn      func(ctx context.Context, dir string, args ...string) gitexec.Result
	execStdinFn func(ctx context.Context, dir, stdin string, args ...string) gitexec.Result
)

type engine struct {
	exec      execFn
	execStdin execStdinFn
	mu        sync.Map
	commonDir sync.Map
}

// repoMutex returns (or creates) the per-repo mutex for repoPath. The mutex is
// keyed by the git common directory, so every worktree of a single clone shares
// one lock and their git operations serialize (07 §3.1). Resolution runs once
// per input path before the lock is taken (no reentrancy), and falls back to the
// raw path when repoPath is not inside a git repo.
func (e *engine) repoMutex(repoPath string) *sync.Mutex {
	key := e.resolveCommonDir(repoPath)
	actual, _ := e.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (e *engine) resolveCommonDir(repoPath string) string {
	if cached, ok := e.commonDir.Load(repoPath); ok {
		return cached.(string)
	}
	key := e.computeCommonDir(repoPath)
	e.commonDir.Store(repoPath, key)
	return key
}

func (e *engine) computeCommonDir(repoPath string) string {
	r := e.exec(context.Background(), repoPath, "rev-parse", "--git-common-dir")
	if gitexec.RequireSuccess("rev-parse --git-common-dir", r) != nil {
		return repoPath
	}
	dir := strings.TrimSpace(r.Stdout)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoPath, dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return repoPath
	}
	if canonical, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		return canonical
	}
	return filepath.Clean(abs)
}

// lockRepo acquires the per-repo mutex and returns an unlock func for use with defer.
func (e *engine) lockRepo(repoPath string) func() {
	mu := e.repoMutex(repoPath)
	mu.Lock()
	return mu.Unlock
}

// New returns a new Engine that shells out to the system git binary.
func New() Engine {
	return &engine{
		exec:      gitexec.Git,
		execStdin: gitexec.GitWithStdin,
	}
}

var _ Engine = (*engine)(nil)

// classifyGitError inspects git output and wraps the appropriate sentinel error.
func classifyGitError(op string, r gitexec.Result) error {
	stderr := strings.ToLower(r.Stderr + r.Stdout)
	base := gitexec.RequireSuccess(op, r)
	if base == nil {
		return nil
	}
	switch {
	case strings.Contains(stderr, "conflict") || strings.Contains(stderr, "merge conflict"):
		return fmt.Errorf("%w: %s", ErrConflict, base)
	case strings.Contains(stderr, "rejected") && strings.Contains(stderr, "non-fast-forward"):
		return fmt.Errorf("%w: %s", ErrRejectedNonFastForward, base)
	case strings.Contains(stderr, "nothing to commit"):
		return fmt.Errorf("%w: %s", ErrNothingToCommit, base)
	case strings.Contains(stderr, "error: your local changes") || strings.Contains(stderr, "please commit or stash"):
		return fmt.Errorf("%w: %s", ErrDirtyTree, base)
	case strings.Contains(stderr, "authentication failed") || strings.Contains(stderr, "could not read username"):
		return fmt.Errorf("%w: %s", ErrAuthFailed, base)
	default:
		return base
	}
}

func (e *engine) Status(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	return status.Parse(ctx, repoPath)
}

func (e *engine) Diff(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	return diff.WorkingTree(ctx, repoPath, staged)
}

func (e *engine) CommitDiff(
	ctx context.Context,
	repoPath string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	return diff.Commit(ctx, repoPath, sha)
}

func (e *engine) Log(
	ctx context.Context,
	repoPath string,
	limit int,
	skip int,
) ([]gitdomain.Commit, error) {
	return gitlog.List(ctx, repoPath, limit, skip)
}

func (e *engine) Blame(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.BlameEntry, error) {
	return blame.File(ctx, repoPath, filePath)
}

func (e *engine) Branches(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Branch, error) {
	return branches.List(ctx, repoPath)
}

func (e *engine) Stashes(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Stash, error) {
	return stash.List(ctx, repoPath)
}

func (e *engine) StageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "add", filePath)
	return gitexec.RequireSuccess("stage file", r)
}

func (e *engine) StageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	defer e.lockRepo(repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, false)
}

func (e *engine) UnstageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "restore", "--staged", filePath)
	return gitexec.RequireSuccess("unstage file", r)
}

func (e *engine) UnstageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	defer e.lockRepo(repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, true)
}

func (e *engine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(repoPath)()
	st, err := e.Status(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("git: discard: status: %w", err)
	}
	for _, f := range st.Files {
		if f.Path != filePath {
			continue
		}
		if f.Status == gitdomain.GitFileStatusUntracked {
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
	r := e.exec(ctx, repoPath, "restore", filePath)
	return gitexec.RequireSuccess("restore", r)
}

func (e *engine) discardUntracked(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	r := e.exec(ctx, repoPath, "clean", "-f", filePath)
	return gitexec.RequireSuccess("clean", r)
}

func (e *engine) Commit(
	ctx context.Context,
	repoPath string,
	subject string,
	body string,
) error {
	defer e.lockRepo(repoPath)()
	args := []string{"commit", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	r := e.exec(ctx, repoPath, args...)
	return classifyGitError("commit", r)
}

func (e *engine) Push(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "push")
	return classifyGitError("push", r)
}

func (e *engine) Fetch(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "fetch")
	return gitexec.RequireSuccess("fetch", r)
}

func (e *engine) Pull(
	ctx context.Context,
	repoPath string,
	mode string,
) error {
	defer e.lockRepo(repoPath)()
	flag := "--no-rebase"
	if mode == "rebase" {
		flag = "--rebase"
	}
	r := e.exec(ctx, repoPath, "pull", flag)
	return classifyGitError("pull", r)
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
	defer e.lockRepo(repoPath)()
	flag := "--" + mode
	r := e.exec(ctx, repoPath, "reset", flag, commit)
	return classifyGitError("reset", r)
}

func (e *engine) Merge(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "merge", branch)
	return classifyGitError("merge", r)
}

func (e *engine) Rebase(
	ctx context.Context,
	repoPath string,
	onto string,
) error {
	defer e.lockRepo(repoPath)()
	r := e.exec(ctx, repoPath, "rebase", onto)
	return classifyGitError("rebase", r)
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
) ([]gitdomain.ConflictHunk, error) {
	return conflicts.ParseFile(ctx, repoPath, filePath)
}

func (e *engine) ResolveHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	resolution gitdomain.ConflictResolution,
	resolvedContent string,
) error {
	return conflicts.ResolveHunk(ctx, repoPath, filePath, hunkID, resolution, resolvedContent)
}

func (e *engine) OperationContinue(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(repoPath)()
	return e.operationContinue(ctx, repoPath)
}

func (e *engine) OperationAbort(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(repoPath)()
	return e.operationAbort(ctx, repoPath)
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
) (gitdomain.GitStatus, error) {
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
	r := e.execStdin(ctx, repoPath, patch, args...)
	return gitexec.RequireSuccess("apply hunk", r)
}
