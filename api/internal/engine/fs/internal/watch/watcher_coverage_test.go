package watch_test

import (
	"context"
	"os"
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

func (c *captureDispatcher) waitForSyncCall(t *testing.T) watch.SyncInput {
	t.Helper()
	var inp watch.SyncInput
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if len(c.syncCalls) == 0 {
			return false
		}
		inp = c.syncCalls[len(c.syncCalls)-1]
		return true
	}, 5*time.Second, 5*time.Millisecond, "timeout: OnSyncWorkingTreeState never called")
	return inp
}

func (c *captureDispatcher) noSyncCallWithin(t *testing.T, d time.Duration) {
	t.Helper()
	// Genuine must-not-happen: assert no sync call appears across the whole window.
	require.Never(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.syncCalls) > 0
	}, d, 20*time.Millisecond, "unexpected OnSyncWorkingTreeState call(s)")
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
	// No settle sleep: Start arms the fsnotify watch synchronously before it
	// returns; callers wait on real observables (dispatched events / git calls).
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

func (g *changingGit) summaryCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWatcher_FanOutGit_SyncCalledWhenSummaryChanges(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	// First write drives recompute #1 (returns 0,0,false,false → no sync, matches
	// the zero-value prev state). Wait for that recompute to actually run before the
	// second write so the two writes don't coalesce into a single recompute.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))
	require.Eventually(t, func() bool {
		return git.summaryCalls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "first git recompute must run before the second write")

	// Second write: changingGit now returns 1,0,false,false → triggers sync
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600))

	inp := d.waitForSyncCall(t)
	assert.Equal(t, 1, inp.Added)
	assert.Equal(t, "ws-cap", inp.WsID)
}

// The rewrite-in-progress suppression of the working-tree summary was removed
// (Bug B, passive path): a conflict created mid-rewrite must still surface. The
// new behavior — summary broadcasts during a rewrite while the per-file status
// storm stays guarded — is proven deterministically by
// TestFanOutGit_DuringRewrite_BroadcastsConflictButGuardsStatusStorm in
// watcher_internal_test.go, which drives fanOutGit directly (no timing).

// ---------------------------------------------------------------------------
// relPath — normal path returns relative value
// ---------------------------------------------------------------------------

func TestWatcher_RelPath_NormalPathIsRelative(t *testing.T) {
	dir := t.TempDir()
	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	filePath := filepath.Join(dir, "rel_test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o600))

	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, evt := range d.fileCalls {
			if evt.Path == "rel_test.txt" {
				return true
			}
		}
		return false
	}, 3*time.Second, 5*time.Millisecond, "expected relative path rel_test.txt in event")
}

// ---------------------------------------------------------------------------
// classifyChange — Rename op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Rename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))

	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.Rename(src, dst))

	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, evt := range d.fileCalls {
			if evt.Type == domain.FileChangeRenamed {
				return true
			}
		}
		return false
	}, 3*time.Second, 5*time.Millisecond, "expected a FileChangeRenamed event from os.Rename")
}

// ---------------------------------------------------------------------------
// classifyChange — Modify op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Modify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.txt")
	require.NoError(t, os.WriteFile(path, []byte("v1"), 0o600))

	_, d := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.WriteFile(path, []byte("v2"), 0o600))

	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, evt := range d.fileCalls {
			if evt.Path == "mod.txt" && evt.Type == domain.FileChangeModified {
				return true
			}
		}
		return false
	}, 3*time.Second, 5*time.Millisecond, "expected FileChangeModified for mod.txt")
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

func (g *commitTogglingGit) summaryCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWatcher_FanOutGit_HasCommitsChange(t *testing.T) {
	dir := t.TempDir()
	git := &commitTogglingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	// Prime write drives recompute #1 (no toggle yet). Wait for it to run before the
	// trigger write so the two do not coalesce into a single recompute.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prime.txt"), []byte("p"), 0o600))
	require.Eventually(t, func() bool {
		return git.summaryCalls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "prime recompute must run before the trigger write")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "commit_trigger.txt"), []byte("c"), 0o600))

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

func (g *conflictTogglingGit) summaryCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWatcher_FanOutGit_HasConflictsChange(t *testing.T) {
	dir := t.TempDir()
	git := &conflictTogglingGit{}
	_, d := newCapturingWatcher(t, dir, git)

	// Prime write drives recompute #1 (no toggle yet). Wait for it to run before the
	// trigger write so the two do not coalesce into a single recompute.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.txt"), []byte("p"), 0o600))
	require.Eventually(t, func() bool {
		return git.summaryCalls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "prime recompute must run before the trigger write")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.txt"), []byte("q"), 0o600))

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

func (g *deletedChangeGit) summaryCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWatcher_FanOutGit_DeletedCountChange(t *testing.T) {
	dir := t.TempDir()
	git := &deletedChangeGit{}
	_, d := newCapturingWatcher(t, dir, git)

	// Prime write drives recompute #1 (no change yet). Wait for it to run before the
	// trigger write so the two do not coalesce into a single recompute.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prime2.txt"), []byte("p"), 0o600))
	require.Eventually(t, func() bool {
		return git.summaryCalls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "prime recompute must run before the trigger write")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "del_trigger.txt"), []byte("d"), 0o600))

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

	require.NoError(t, os.WriteFile(filepath.Join(dir, "err_test.txt"), []byte("e"), 0o600))

	d.noSyncCallWithin(t, 400*time.Millisecond)
}

// ---------------------------------------------------------------------------
// fanOutGit — ComputeStatus error path (no git/sync dispatched at all)
// ---------------------------------------------------------------------------

type statusErrorGit struct {
	mu    sync.Mutex
	calls int
}

func (g *statusErrorGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	return gitdomain.GitStatus{}, assert.AnError
}

func (g *statusErrorGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

func (g *statusErrorGit) statusCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWatcher_FanOutGit_StatusErrorNoGitCall(t *testing.T) {
	dir := t.TempDir()
	git := &statusErrorGit{}
	d := &captureDispatcher{}
	w := watch.NewWatcher("ws-err", dir, "", git, d)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat_err.txt"), []byte("s"), 0o600))

	// Wait until the (error-returning) ComputeStatus has actually been attempted,
	// so the assertion below is not vacuous: the recompute fired but OnGitStatus was
	// suppressed. ComputeStatus errors BEFORE OnGitStatus is ever reached, so
	// gitCalls can never become >0 for this recompute.
	require.Eventually(t, func() bool {
		return git.statusCalls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "ComputeStatus must be attempted after a file change")

	d.mu.Lock()
	defer d.mu.Unlock()
	assert.Equal(t, 0, d.gitCalls, "OnGitStatus should not be called when ComputeStatus fails")
}

// The rebase-merge / rebase-apply summary-suppression tests were removed for the
// same reason as the MERGE_HEAD one above: the working-tree summary now always
// broadcasts so conflicts surface. isRewriteInProgress's handling of all three
// markers is unit-tested in watcher_internal_test.go (TestIsRewriteInProgress_*),
// and the mid-rewrite fanOutGit behavior is proven there deterministically.
