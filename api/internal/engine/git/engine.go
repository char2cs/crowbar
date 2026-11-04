package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/blame"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/branches"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/conflicts"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	gitlog "github.com/char2cs/crowbar/api/internal/engine/git/internal/log"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/stash"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/status"
	"github.com/char2cs/crowbar/api/internal/perf"
)

type (
	execFn      func(ctx context.Context, dir string, args ...string) gitexec.Result
	execStdinFn func(ctx context.Context, dir, stdin string, args ...string) gitexec.Result
)

type engine struct {
	exec      execFn
	execStdin execStdinFn
	mu        sync.Map // key: common dir (string) -> *sync.RWMutex
	commonDir sync.Map
	verdicts  mergeVerdicts
	tips      tipMemory
}

// repoMutex returns (or creates) the per-repo mutex for repoPath, keyed by the git common dir so every
// worktree of a clone serializes on one lock. It falls back to the raw path outside a git repo.
func (e *engine) repoMutex(ctx context.Context, repoPath string) *sync.RWMutex {
	key := e.resolveCommonDir(ctx, repoPath)
	actual, _ := e.mu.LoadOrStore(key, &sync.RWMutex{})
	return actual.(*sync.RWMutex)
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

// lockRepo takes the exclusive write lock for repoPath's clone, for any
// operation that mutates the working tree, index, refs, or touches the
// network (07 §3.1).
func (e *engine) lockRepo(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	if !perf.Enabled() {
		mu.Lock()
		return mu.Unlock
	}
	waitStart := time.Now()
	mu.Lock()
	perf.Record("lock.write.wait", time.Since(waitStart))
	held := time.Now()
	return func() {
		perf.Record("lock.write.hold", time.Since(held))
		mu.Unlock()
	}
}

// lockRepoRead takes the clone's shared read lock for read-only inspection. Reads never block each
// other, only writes (including origin-sync fetches), and never observe a torn mutation.
func (e *engine) lockRepoRead(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	if !perf.Enabled() {
		mu.RLock()
		return mu.RUnlock
	}
	waitStart := time.Now()
	mu.RLock()
	perf.Record("lock.read.wait", time.Since(waitStart))
	held := time.Now()
	return func() {
		perf.Record("lock.read.hold", time.Since(held))
		mu.RUnlock()
	}
}

// New returns a new Engine that shells out to the system git binary.
func New() Engine {
	return &engine{
		exec:      gitexec.Git,
		execStdin: gitexec.GitWithStdin,
	}
}

var _ Engine = (*engine)(nil)

// netTransferTimeout and netQueryTimeout bound remote git calls so a dead TCP connection cannot hold the
// per-repo mutex for the OS retransmission timeout. netListRefreshTimeout is far tighter: a user waits on
// it, and failure only leaves the listing on cached remote-tracking refs.
var (
	netTransferTimeout    = 3 * time.Minute
	netQueryTimeout       = 30 * time.Second
	netListRefreshTimeout = 45 * time.Second
)

// execNet runs a network git command under timeout, reporting a deadline kill as an explicit timeout
// error instead of exit -1; a cancelled parent context keeps ordinary classification.
func (e *engine) execNet(ctx context.Context, op string, timeout time.Duration, repoPath string, args ...string) error {
	r, err := e.runNet(ctx, op, timeout, repoPath, args...)
	if err != nil {
		return err
	}
	return classifyGitError(op, r)
}

// runNet is execNet before classification; its error is only the timeout.
func (e *engine) runNet(
	ctx context.Context,
	op string,
	timeout time.Duration,
	repoPath string,
	args ...string,
) (gitexec.Result, error) {
	nctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r := e.exec(nctx, repoPath, args...)
	if errors.Is(nctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return r, &gitexec.GitError{
			Op:       op,
			ExitCode: r.ExitCode,
			Message:  fmt.Sprintf("network operation timed out after %s", timeout),
		}
	}
	return r, nil
}

func (e *engine) Status(ctx context.Context, repoPath string) (gitdomain.GitStatus, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return status.Parse(ctx, repoPath)
}

func (e *engine) Diff(ctx context.Context, repoPath string, staged bool) ([]gitdomain.FileDiff, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.WorkingTree(ctx, repoPath, staged)
}

func (e *engine) CommitDiff(ctx context.Context, repoPath, sha string) (gitdomain.MultiFileDiff, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.Commit(ctx, repoPath, sha)
}

func (e *engine) Log(ctx context.Context, repoPath string, limit, skip int) ([]gitdomain.Commit, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return gitlog.List(ctx, repoPath, limit, skip)
}

func (e *engine) Blame(ctx context.Context, repoPath, filePath string) ([]gitdomain.BlameEntry, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return blame.File(ctx, repoPath, filePath)
}

func (e *engine) Branches(ctx context.Context, repoPath string) ([]gitdomain.Branch, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return branches.List(ctx, repoPath)
}

func (e *engine) Stashes(ctx context.Context, repoPath string) ([]gitdomain.Stash, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return stash.List(ctx, repoPath)
}

func (e *engine) StageFile(ctx context.Context, repoPath, filePath string) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "add", filePath)
	return gitexec.RequireSuccess("stage file", r)
}

func (e *engine) StageHunk(ctx context.Context, repoPath, filePath, hunkID string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, false)
}

func (e *engine) UnstageFile(ctx context.Context, repoPath, filePath string) error {
	defer e.lockRepo(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "restore", "--staged", filePath)
	return gitexec.RequireSuccess("unstage file", r)
}

func (e *engine) UnstageHunk(ctx context.Context, repoPath, filePath, hunkID string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.applyHunk(ctx, repoPath, filePath, hunkID, true)
}

func (e *engine) Discard(ctx context.Context, repoPath, filePath string) error {
	defer e.lockRepo(ctx, repoPath)()
	// Call status.Parse directly, NOT e.Status: e.Status now takes RLock, and
	// this method already holds the exclusive Lock — Go's sync.RWMutex is not
	// reentrant, so going through e.Status here would deadlock.
	st, err := status.Parse(ctx, repoPath)
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

func (e *engine) discardTracked(ctx context.Context, repoPath, filePath string) error {
	r := e.exec(ctx, repoPath, "restore", filePath)
	return gitexec.RequireSuccess("restore", r)
}

func (e *engine) discardUntracked(ctx context.Context, repoPath, filePath string) error {
	r := e.exec(ctx, repoPath, "clean", "-f", filePath)
	return gitexec.RequireSuccess("clean", r)
}

func (e *engine) Commit(ctx context.Context, repoPath, subject, body string) error {
	defer e.lockRepo(ctx, repoPath)()
	args := []string{"commit", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	r := e.exec(ctx, repoPath, args...)
	return classifyGitError("commit", r)
}

func (e *engine) Push(ctx context.Context, repoPath string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.execNet(ctx, "push", netTransferTimeout, repoPath, "push")
}

func (e *engine) Fetch(ctx context.Context, repoPath string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.execNet(ctx, "fetch", netTransferTimeout, repoPath, "fetch")
}

func (e *engine) FetchRef(ctx context.Context, repoPath, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.execNet(ctx, "fetch ref", netTransferTimeout, repoPath, "fetch", "origin", branch)
}

// FetchPrune runs `git fetch --prune origin` so a remote-branch listing reflects the remote now rather
// than at clone time; nothing else fetches all refs. It touches no local branch, so reads may run it.
func (e *engine) FetchPrune(ctx context.Context, repoPath string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.execNet(ctx, "fetch prune", netListRefreshTimeout, repoPath, "fetch", "--prune", "origin")
}

// FastForwardBranch runs `git fetch origin <branch>:<branch>`, updating the local branch (not just the
// tracking ref) before it is checked out into a worktree.
func (e *engine) FastForwardBranch(ctx context.Context, repoPath, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.execNet(ctx, "fast-forward branch", netTransferTimeout, repoPath, "fetch", "origin", branch+":"+branch)
}

func (e *engine) CreateBranch(ctx context.Context, repoPath, name, source string, switchTo bool) error {
	return reclassifyError(branches.Create(ctx, repoPath, name, source, switchTo))
}

func (e *engine) RenameBranch(ctx context.Context, repoPath, oldName, newName string) error {
	return reclassifyError(branches.Rename(ctx, repoPath, oldName, newName))
}

func (e *engine) DeleteBranch(ctx context.Context, repoPath, name string) error {
	return branches.Delete(ctx, repoPath, name)
}

func (e *engine) ForceDeleteBranch(ctx context.Context, repoPath, name string) error {
	return branches.ForceDelete(ctx, repoPath, name)
}

func (e *engine) SwitchBranch(ctx context.Context, repoPath, name string) error {
	return reclassifyError(branches.Switch(ctx, repoPath, name))
}

func (e *engine) StashPush(ctx context.Context, repoPath, message string) error {
	return stash.Push(ctx, repoPath, message)
}

func (e *engine) StashApply(ctx context.Context, repoPath, id string) error {
	return stash.Apply(ctx, repoPath, id)
}

func (e *engine) StashPop(ctx context.Context, repoPath, id string) error {
	return stash.Pop(ctx, repoPath, id)
}

func (e *engine) StashDrop(ctx context.Context, repoPath, id string) error {
	return stash.Drop(ctx, repoPath, id)
}

// Reset runs `git reset --<mode> [<commit>]`. git reset rejects a `--` before a commit, so the guard is
// the usecase boundary's mode allowlist and leading-dash check on commit.
func (e *engine) Reset(ctx context.Context, repoPath, mode, commit string) error {
	defer e.lockRepo(ctx, repoPath)()
	flag := "--" + mode
	args := []string{"reset", flag}
	if commit != "" {
		args = append(args, commit)
	}
	r := e.exec(ctx, repoPath, args...)
	return classifyGitError("reset", r)
}

func (e *engine) ConflictedFiles(ctx context.Context, repoPath string) ([]string, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return conflicts.ConflictedFiles(ctx, repoPath)
}

func (e *engine) ConflictHunks(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	defer e.lockRepoRead(ctx, repoPath)()
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

func (e *engine) OperationContinue(ctx context.Context, repoPath string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.operationContinue(ctx, repoPath)
}

func (e *engine) OperationAbort(ctx context.Context, repoPath string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.operationAbort(ctx, repoPath)
}

// OperationInProgress reports the in-progress git operation from .git marker files only, taking no repo
// lock: recovery calls it on worktrees that may be mid-operation.
func (e *engine) OperationInProgress(ctx context.Context, repoPath string) (string, error) {
	return detectInProgressOp(ctx, repoPath), nil
}

func (e *engine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return e.computeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}

// ComputeStatus implements watch.GitStatusProvider.
func (e *engine) ComputeStatus(ctx context.Context, repoPath string) (gitdomain.GitStatus, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return status.Parse(ctx, repoPath)
}

// ResolveDiffBase implements watch.GitStatusProvider.
func (e *engine) ResolveDiffBase(ctx context.Context, repoPath, base string) gitdomain.DiffBase {
	defer e.lockRepoRead(ctx, repoPath)()
	return e.diffBase(ctx, repoPath, base)
}

// DiffStat implements watch.GitStatusProvider: the +N/-N of the working tree
// against an already-resolved commit.
func (e *engine) DiffStat(ctx context.Context, repoPath, commit string) (int, int, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return e.numstatFromBase(ctx, repoPath, commit)
}

func (e *engine) applyHunk(ctx context.Context, repoPath, filePath, hunkID string, reverse bool) error {
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
