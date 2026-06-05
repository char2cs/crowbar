package watch_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// captureDispatcher records every callback so tests can poll for results.
type captureDispatcher struct {
	mu        sync.Mutex
	gitCalls  int
	syncCalls []watch.SyncInput
	fileCalls []domain.FileChangeEvent
}

func (c *captureDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fileCalls = append(c.fileCalls, evt)
}

func (c *captureDispatcher) OnGitStatus(
	_ context.Context,
	_ string,
	_ gitdomain.GitStatus,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gitCalls++
}

func (c *captureDispatcher) OnSyncWorkingTreeState(
	_ context.Context,
	input watch.SyncInput,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncCalls = append(c.syncCalls, input)
}

func (c *captureDispatcher) waitForGitCall(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := c.gitCalls
		c.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout: OnGitStatus never called")
}

func (c *captureDispatcher) waitForSyncCall(t *testing.T) watch.SyncInput {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.syncCalls)
		c.mu.Unlock()
		if n > 0 {
			c.mu.Lock()
			inp := c.syncCalls[len(c.syncCalls)-1]
			c.mu.Unlock()
			return inp
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout: OnSyncWorkingTreeState never called")
	return watch.SyncInput{}
}

func (c *captureDispatcher) noSyncCallWithin(t *testing.T, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.syncCalls)
		c.mu.Unlock()
		if n > 0 {
			t.Errorf("unexpected OnSyncWorkingTreeState call(s)")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// gitCmd runs a git command in dir and fails the test on error.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// initGitRepo creates a real, minimal git repo with an initial commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

// newCapturingWatcher creates a Watcher with captureDispatcher, starts it, and registers cleanup.
func newCapturingWatcher(
	t *testing.T,
	dir string,
	git watch.GitStatusProvider,
) (*watch.Watcher, *captureDispatcher) {
	t.Helper()
	d := &captureDispatcher{}
	w := watch.NewWatcher("ws-cap", dir, "", git, d)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
	// Small settle so the watcher is fully registered before we write.
	time.Sleep(60 * time.Millisecond)
	return w, d
}

// ---------------------------------------------------------------------------
// maybeHandleGitRef and shouldIgnore
// ---------------------------------------------------------------------------
// These paths are exercised in watcher_internal_test.go via direct method
// calls, because macOS kqueue does not report fsnotify events for files
// inside a .git/ directory when only the parent dir is watched.

// ---------------------------------------------------------------------------
// fanOutGit — OnSyncWorkingTreeState fires when summary changes
// ---------------------------------------------------------------------------

// changingGit returns different values on successive ComputeWorkingTreeSummary
// calls, exercising the "summary changed" branch of fanOutGit.
type changingGit struct {
	mu    sync.Mutex
	calls int
}

func (g *changingGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *changingGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	// First call: 0,0,false,false (matches zero-value prev state — no sync)
	// Second+ call: 1,0,false,false → triggers OnSyncWorkingTreeState
	if g.calls >= 2 {
		return 1, 0, false, false, nil
	}
	return 0, 0, false, false, nil
}

func TestWatcher_FanOutGit_SyncCalledWhenSummaryChanges(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	// First write: changingGit returns 0,0,false,false → no sync (matches initial state)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0600))
	time.Sleep(300 * time.Millisecond)

	// Second write: changingGit now returns 1,0,false,false → triggers sync
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0600))

	inp := d.waitForSyncCall(t)
	assert.Equal(t, 1, inp.Added)
	assert.Equal(t, "ws-cap", inp.WsID)
}

// ---------------------------------------------------------------------------
// fanOutGit — MERGE_HEAD suppresses OnSyncWorkingTreeState
// ---------------------------------------------------------------------------

func TestWatcher_FanOutGit_MergeHeadSuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(gitDir, "MERGE_HEAD"),
		[]byte("deadbeef\n"),
		0600,
	))

	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "y.txt"), []byte("y"), 0600))

	d.noSyncCallWithin(t, 400*time.Millisecond)
}

// ---------------------------------------------------------------------------
// relPath — normal path returns relative value
// ---------------------------------------------------------------------------

func TestWatcher_RelPath_NormalPathIsRelative(t *testing.T) {
	dir := t.TempDir()
	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	filePath := filepath.Join(dir, "rel_test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0600))

	deadline := time.Now().Add(3 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		d.mu.Lock()
		for _, evt := range d.fileCalls {
			if evt.Path == "rel_test.txt" {
				found = true
			}
		}
		d.mu.Unlock()
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, found, "expected relative path rel_test.txt in event")
}

// ---------------------------------------------------------------------------
// classifyChange — Rename op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Rename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0600))

	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.Rename(src, dst))

	deadline := time.Now().Add(3 * time.Second)
	var renameFound bool
	for time.Now().Before(deadline) {
		d.mu.Lock()
		for _, evt := range d.fileCalls {
			if evt.Type == domain.FileChangeRenamed {
				renameFound = true
			}
		}
		d.mu.Unlock()
		if renameFound {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, renameFound, "expected a FileChangeRenamed event from os.Rename")
}

// ---------------------------------------------------------------------------
// classifyChange — Modify op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Modify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.txt")
	require.NoError(t, os.WriteFile(path, []byte("v1"), 0600))

	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.WriteFile(path, []byte("v2"), 0600))

	deadline := time.Now().Add(3 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		d.mu.Lock()
		for _, evt := range d.fileCalls {
			if evt.Path == "mod.txt" && evt.Type == domain.FileChangeModified {
				found = true
			}
		}
		d.mu.Unlock()
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, found, "expected FileChangeModified for mod.txt")
}

// ---------------------------------------------------------------------------
// fanOutGit — hasCommits toggling (false → true)
// ---------------------------------------------------------------------------

type commitTogglingGit struct {
	mu    sync.Mutex
	calls int
}

func (g *commitTogglingGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *commitTogglingGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls >= 2 {
		return 0, 0, false, true, nil
	}
	return 0, 0, false, false, nil
}

func TestWatcher_FanOutGit_HasCommitsChange(t *testing.T) {
	dir := t.TempDir()
	git := &commitTogglingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "prime.txt"), []byte("p"), 0600))
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "commit_trigger.txt"), []byte("c"), 0600))

	inp := d.waitForSyncCall(t)
	assert.True(t, inp.HasCommits)
}

// ---------------------------------------------------------------------------
// fanOutGit — hasConflicts toggling
// ---------------------------------------------------------------------------

type conflictTogglingGit struct {
	mu    sync.Mutex
	calls int
}

func (g *conflictTogglingGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *conflictTogglingGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls >= 2 {
		return 0, 0, true, false, nil
	}
	return 0, 0, false, false, nil
}

func TestWatcher_FanOutGit_HasConflictsChange(t *testing.T) {
	dir := t.TempDir()
	git := &conflictTogglingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.txt"), []byte("p"), 0600))
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.txt"), []byte("q"), 0600))

	inp := d.waitForSyncCall(t)
	assert.True(t, inp.HasConflicts)
}

// ---------------------------------------------------------------------------
// fanOutGit — deleted count change
// ---------------------------------------------------------------------------

type deletedChangeGit struct {
	mu    sync.Mutex
	calls int
}

func (g *deletedChangeGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *deletedChangeGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls >= 2 {
		return 0, 3, false, false, nil
	}
	return 0, 0, false, false, nil
}

func TestWatcher_FanOutGit_DeletedCountChange(t *testing.T) {
	dir := t.TempDir()
	git := &deletedChangeGit{}
	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "prime2.txt"), []byte("p"), 0600))
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "del_trigger.txt"), []byte("d"), 0600))

	inp := d.waitForSyncCall(t)
	assert.Equal(t, 3, inp.Deleted)
}

// ---------------------------------------------------------------------------
// fanOutGit — ComputeWorkingTreeSummary error path (no sync dispatched)
// ---------------------------------------------------------------------------

type summaryErrorGit struct{}

func (g *summaryErrorGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *summaryErrorGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, assert.AnError
}

func TestWatcher_FanOutGit_SummaryErrorNoSync(t *testing.T) {
	dir := t.TempDir()
	_, d := newCapturingWatcher(t, dir, &summaryErrorGit{})

	require.NoError(t, os.WriteFile(filepath.Join(dir, "err_test.txt"), []byte("e"), 0600))

	d.noSyncCallWithin(t, 400*time.Millisecond)
}

// ---------------------------------------------------------------------------
// fanOutGit — ComputeStatus error path (no git/sync dispatched at all)
// ---------------------------------------------------------------------------

type statusErrorGit struct{}

func (g *statusErrorGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, assert.AnError
}

func (g *statusErrorGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

func TestWatcher_FanOutGit_StatusErrorNoGitCall(t *testing.T) {
	dir := t.TempDir()
	d := &captureDispatcher{}
	w := watch.NewWatcher("ws-err", dir, "", &statusErrorGit{}, d)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
	time.Sleep(60 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat_err.txt"), []byte("s"), 0600))

	time.Sleep(400 * time.Millisecond)
	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Equal(t, 0, d.gitCalls, "OnGitStatus should not be called when ComputeStatus fails")
}

// ---------------------------------------------------------------------------
// isRewriteInProgress — rebase-merge and rebase-apply also suppress sync
// ---------------------------------------------------------------------------

func TestWatcher_FanOutGit_RebaseMergeSuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0700))

	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "rb.txt"), []byte("r"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rb2.txt"), []byte("r2"), 0600))

	d.noSyncCallWithin(t, 400*time.Millisecond)
}

func TestWatcher_FanOutGit_RebaseApplySuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-apply"), 0700))

	_, d := newCapturingWatcher(t, dir, git)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ra.txt"), []byte("r"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ra2.txt"), []byte("r2"), 0600))

	d.noSyncCallWithin(t, 400*time.Millisecond)
}
