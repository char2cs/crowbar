package git

import (
	"context"
	"errors"
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
func (e *engine) repoMutex(ctx context.Context, repoPath string) *sync.Mutex {
	key := e.resolveCommonDir(ctx, repoPath)
	actual, _ := e.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (e *engine) resolveCommonDir(ctx context.Context, repoPath string) string {
	if cached, ok := e.commonDir.Load(repoPath); ok {
		return cached.(string)
	}
	key := e.computeCommonDir(ctx, repoPath)
	e.commonDir.Store(repoPath, key)
	return key
}

func (e *engine) computeCommonDir(ctx context.Context, repoPath string) string {
	r := e.exec(ctx, repoPath, "rev-parse", "--git-common-dir")
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

func (e *engine) lockRepo(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
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

// errorRule maps a git exit code + output substrings to a sentinel error.
// All patterns in contains must be present in the output (AND semantics).
// Rules are evaluated in order; first match wins.
type errorRule struct {
	exitCode int
	contains []string
	sentinel error
}

var errorRules = []errorRule{
	// Exit 1 — soft failure: git ran but reported a condition.
	{1, []string{"conflict"}, ErrConflict},
	{1, []string{"nothing to commit"}, ErrNothingToCommit},
	{1, []string{"rejected", "non-fast-forward"}, ErrRejectedNonFastForward},
	{1, []string{"not possible to fast-forward"}, ErrNonFastForward},
	{1, []string{"your local changes"}, ErrDirtyTree},
	{1, []string{"please commit or stash"}, ErrDirtyTree},

	// Exit 128 — fatal: git refused to execute.
	{128, []string{"authentication failed"}, ErrAuthFailed},
	{128, []string{"could not read username"}, ErrAuthFailed},
	{128, []string{"already exists"}, ErrBranchAlreadyExists},
	{128, []string{"already checked out"}, ErrBranchAlreadyExists},
	{128, []string{"already used by worktree"}, ErrBranchAlreadyExists},
	{128, []string{"did not match any"}, ErrBranchNotFound},
	{128, []string{"unknown revision or path"}, ErrBranchNotFound},
	{128, []string{"does not appear to be a git repository"}, ErrNoRemote},
	{128, []string{"no remote configured"}, ErrNoRemote},
	{128, []string{"refusing to merge unrelated histories"}, ErrNonFastForward},
	{128, []string{"your local changes"}, ErrDirtyTree},
	{128, []string{"please commit or stash"}, ErrDirtyTree},
}

// matchRules returns the first sentinel whose exit code and all patterns match,
// or nil if no rule applies.
func matchRules(exitCode int, out string) error {
	lower := strings.ToLower(out)
	for _, r := range errorRules {
		if r.exitCode != exitCode {
			continue
		}
		if allPresent(lower, r.contains) {
			return r.sentinel
		}
	}
	return nil
}

func allPresent(s string, patterns []string) bool {
	for _, p := range patterns {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

// classifyGitError maps a git subprocess result to a typed sentinel using
// exit code as the primary discriminator and output text only to disambiguate
// within the same exit code.
func classifyGitError(op string, r gitexec.Result) error {
	base := gitexec.RequireSuccess(op, r)
	if base == nil {
		return nil
	}
	if sentinel := matchRules(r.ExitCode, r.Stderr+r.Stdout); sentinel != nil {
		return fmt.Errorf("%w: %s", sentinel, base)
	}
	return base
}

// reclassifyError applies the same rule table to an error returned by an
// internal subpackage (e.g. branches) that manages its own exec calls.
// It uses errors.As to extract the exit code directly from the GitError struct.
func reclassifyError(err error) error {
	if err == nil {
		return nil
	}
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return err
	}
	if sentinel := matchRules(gitErr.ExitCode, gitErr.Message); sentinel != nil {
		return fmt.Errorf("%w: %s", sentinel, err)
	}
	return err
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
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "add", filePath)
	return gitexec.RequireSuccess("stage file", r)
}

func (e *engine) StageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, false)
}

func (e *engine) UnstageFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "restore", "--staged", filePath)
	return gitexec.RequireSuccess("unstage file", r)
}

func (e *engine) UnstageHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, true)
}

func (e *engine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
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
	defer e.lockRepo(ctx, repoPath)()
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
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "push")
	return classifyGitError("push", r)
}

func (e *engine) Fetch(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "fetch")
	return classifyGitError("fetch", r)
}

func (e *engine) FetchRef(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "fetch", "origin", branch)
	return classifyGitError("fetch ref", r)
}

// FastForwardBranch fetches origin/<branch> and fast-forwards the local branch
// ref to match it in one step (`git fetch origin <branch>:<branch>`). Unlike
// FetchRef, which only updates the remote-tracking ref, this ensures the local
// branch is up to date before checking it out into a worktree.
func (e *engine) FastForwardBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "fetch", "origin", branch+":"+branch)
	return classifyGitError("fast-forward branch", r)
}

func (e *engine) Pull(
	ctx context.Context,
	repoPath string,
	mode string,
) error {
	defer e.lockRepo(ctx, repoPath)()
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
	return reclassifyError(branches.Create(ctx, repoPath, name, source, switchTo))
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
	return reclassifyError(branches.Switch(ctx, repoPath, name))
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

// Reset runs `git reset --<mode> [<commit>]`. mode is allowlisted and commit is
// validated for a leading dash at the usecase boundary (git_write.go), which is
// the guard here: `git reset` does not accept a `--` end-of-options separator
// before a commit (it would reinterpret the commit as a pathspec — "Cannot do
// hard reset with paths"), so a separator is infeasible for this subcommand.
func (e *engine) Reset(
	ctx context.Context,
	repoPath string,
	mode string,
	commit string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	flag := "--" + mode
	args := []string{"reset", flag}
	if commit != "" {
		args = append(args, commit)
	}
	r := e.exec(ctx, repoPath, args...)
	return classifyGitError("reset", r)
}

func (e *engine) Merge(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	// `--` end-of-options separator so a branch value can never be read as a git
	// option (argument injection); the usecase boundary also rejects leading-dash
	// operands as defense in depth.
	r := e.exec(ctx, repoPath, "merge", "--", branch)
	return classifyGitError("merge", r)
}

func (e *engine) Rebase(
	ctx context.Context,
	repoPath string,
	onto string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	// `--` end-of-options separator so an onto value can never be read as a git
	// option such as `--exec=<cmd>` (argument injection → arbitrary command
	// execution); the usecase boundary also rejects leading-dash operands.
	r := e.exec(ctx, repoPath, "rebase", "--", onto)
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
	defer e.lockRepo(ctx, repoPath)()
	return e.operationContinue(ctx, repoPath)
}

func (e *engine) OperationAbort(
	ctx context.Context,
	repoPath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.operationAbort(ctx, repoPath)
}

// OperationInProgress reports the in-progress git operation at repoPath, reading
// only the .git marker files. It takes no repo lock: the marker check is a pure
// read and the recovery sweep calls it on workspaces whose worktrees may be mid
// operation, where blocking on a write lock would be wrong.
func (e *engine) OperationInProgress(
	ctx context.Context,
	repoPath string,
) (string, error) {
	return detectInProgressOp(ctx, repoPath), nil
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
