package watch_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// ---------------------------------------------------------------------------
// Test seams: every wait in this package blocks on a real signal — a dispatched
// callback, a git provider invocation, a completed recompute — never on a clock.
// ---------------------------------------------------------------------------

// manualGitTimer is a watch.GitTimer that never fires on its own: only Fire
// releases it. It lets a test prove the trailing git-recompute debounce
// *coalesces* (N bursts arm the timer, one fire yields exactly one recompute)
// without racing the real 250ms window, and lets the fanOutGit tests run a
// recompute the instant they want one.
type manualGitTimer struct {
	mu   sync.Mutex
	arms int
	c    chan time.Time
}

func newManualGitTimer() *manualGitTimer {
	return &manualGitTimer{c: make(chan time.Time, 1)}
}

// Reset records an arm. The timer still never fires by itself.
func (m *manualGitTimer) Reset(
	_ time.Duration,
) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arms++
	return true
}

func (m *manualGitTimer) Stop() bool {
	return true
}

func (m *manualGitTimer) Chan() <-chan time.Time {
	return m.c
}

// Fire releases the debounce exactly once; the watcher's loop then runs one
// git recompute.
func (m *manualGitTimer) Fire() {
	m.c <- time.Time{}
}

func (m *manualGitTimer) armCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.arms
}

type fakeGit struct{}

func (f *fakeGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (f *fakeGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

type fakeDispatcher struct {
	mu      sync.Mutex
	changes []domain.FileChangeEvent
	// pulse carries one token per dispatched callback (coalescing, buffered 1).
	// Waiters re-scan the recorded events on every token, so no token can be
	// missed and no waiter needs a poll interval.
	pulse chan struct{}
}

func newFakeDispatcher() *fakeDispatcher {
	return &fakeDispatcher{pulse: make(chan struct{}, 1)}
}

func (f *fakeDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	f.mu.Lock()
	f.changes = append(f.changes, evt)
	f.mu.Unlock()
	select {
	case f.pulse <- struct{}{}:
	default:
	}
}

func (f *fakeDispatcher) OnGitStatus(
	_ context.Context,
	_ string,
	_ gitdomain.GitStatus,
) {
}

func (f *fakeDispatcher) OnSyncWorkingTreeState(
	_ context.Context,
	_ watch.SyncInput,
) {
}

func (f *fakeDispatcher) find(
	match func(domain.FileChangeEvent) bool,
) (domain.FileChangeEvent, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, evt := range f.changes {
		if match(evt) {
			return evt, true
		}
	}
	return domain.FileChangeEvent{}, false
}

// waitForChange blocks until a dispatched file-change event satisfies match.
// The wait is on the watcher's own callback — the real arrival of the fsnotify
// event through the debounce — with no deadline: `go test -timeout` is the only
// backstop, and a watcher that never dispatches is a bug, not a slow machine.
func (f *fakeDispatcher) waitForChange(
	match func(domain.FileChangeEvent) bool,
) domain.FileChangeEvent {
	for {
		if evt, ok := f.find(match); ok {
			return evt
		}
		<-f.pulse
	}
}

func (f *fakeDispatcher) hasChange(
	match func(domain.FileChangeEvent) bool,
) bool {
	_, ok := f.find(match)
	return ok
}

// pathIs matches a dispatched event by its workspace-relative path.
func pathIs(
	rel string,
) func(domain.FileChangeEvent) bool {
	return func(evt domain.FileChangeEvent) bool {
		return evt.Path == rel
	}
}

func newWatcher(
	t *testing.T,
	dir string,
	wsID string,
) (*watch.Watcher, *fakeDispatcher) {
	t.Helper()
	d := newFakeDispatcher()
	w := watch.NewWatcher(wsID, dir, "", &fakeGit{}, d)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
	// No settle sleep: Start arms the fsnotify watch synchronously (addRecursive
	// runs before Start returns), so any event emitted after this point is queued
	// by the kernel and delivered to loop; callers wait on real observables.
	return w, d
}

func TestWatcher_DetectsFileCreate(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws1")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o600))

	evt := d.waitForChange(pathIs("new.txt"))
	assert.Equal(t, domain.FileChangeCreated, evt.Type)
	assert.Equal(t, "new.txt", evt.Path)
	assert.Equal(t, "ws1", evt.WsID)
}

func TestWatcher_DetectsFileModify(
	t *testing.T,
) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))

	_, d := newWatcher(t, dir, "ws2")

	require.NoError(t, os.WriteFile(path, []byte("modified"), 0o600))

	_ = d.waitForChange(pathIs("existing.txt"))
}

func TestWatcher_DetectsFileDelete(
	t *testing.T,
) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todelete.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

	_, d := newWatcher(t, dir, "ws3")

	require.NoError(t, os.Remove(path))

	evt := d.waitForChange(func(e domain.FileChangeEvent) bool {
		return e.Path == "todelete.txt" && e.Type == domain.FileChangeDeleted
	})
	assert.Equal(t, domain.FileChangeDeleted, evt.Type)
}

func TestWatcher_IgnoresDotGitContent(
	t *testing.T,
) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))

	_, d := newWatcher(t, dir, "ws4")

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "COMMIT_EDITMSG"), []byte("msg"), 0o600))

	// Barrier, not a window. .git/ is never added to the fsnotify watch, so the
	// write above can only ever surface through the same Events channel and the
	// same (single-goroutine, in-arrival-order) burst handler as the sentinel
	// written after it. Once the sentinel has been dispatched, any COMMIT_EDITMSG
	// event that was ever going to be dispatched already has been — so this is a
	// real signal, not "nothing happened for 250ms".
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("s"), 0o600))
	_ = d.waitForChange(pathIs("sentinel.txt"))

	assert.False(t, d.hasChange(func(evt domain.FileChangeEvent) bool {
		return strings.Contains(evt.Path, "COMMIT_EDITMSG")
	}), "COMMIT_EDITMSG must never surface as a file-change event")
}

func TestWatcher_StopIdempotent(
	t *testing.T,
) {
	dir := t.TempDir()
	w := watch.NewWatcher("ws5", dir, "", &fakeGit{}, newFakeDispatcher())
	require.NoError(t, w.Start(context.Background()))
	w.Stop()
	w.Stop()
	// Stop is complete only once the loop goroutine is gone; block on its own
	// exit signal rather than assuming it.
	<-w.LoopDoneForTest()
}

func TestWatcher_SubdirCreatedAndWatched(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws6")

	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o600))

	// The subdir's own CREATE event on the watched repo root is the real signal
	// that the watcher processed the burst (and, in handleOne, added the new
	// directory to the watch).
	_ = d.waitForChange(pathIs("subdir"))
}

// countingGit counts ComputeStatus invocations and signals each one, so tests
// can block on "the recompute ran" instead of polling a counter.
type countingGit struct {
	mu          sync.Mutex
	statusCalls int
	called      chan struct{}
}

func newCountingGit() *countingGit {
	return &countingGit{called: make(chan struct{}, 16)}
}

func (g *countingGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	g.mu.Lock()
	g.statusCalls++
	g.mu.Unlock()
	select {
	case g.called <- struct{}{}:
	default:
	}
	return gitdomain.GitStatus{}, nil
}

func (g *countingGit) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

func (g *countingGit) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.statusCalls
}

// Git recomputes are trailing-debounced: several fs-event bursts in quick
// succession must coalesce into a single fanOutGit run (one ComputeStatus).
// Without the debounce, each of the four bursts below would shell out to git on
// its own (the ~6Hz churn observed with linked worktrees sharing one .git).
//
// The debounce timer is replaced by one this test fires by hand, so coalescing
// is proven exactly — four bursts arm it, zero recomputes run, one fire yields
// exactly one recompute — instead of asserting "not too many recomputes within
// 400ms", which is a race against the scheduler, not a test of the watcher.
func TestWatcher_GitRecomputeDebounced_CoalescesBursts(
	t *testing.T,
) {
	dir := t.TempDir()
	git := newCountingGit()
	d := newFakeDispatcher()
	w := watch.NewWatcher("ws-debounce", dir, "", git, d)

	timer := newManualGitTimer()
	w.SetGitTimerForTest(timer)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	const bursts = 4
	for i := range bursts {
		rel := fmt.Sprintf("burst%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte("x"), 0o600))
		// Sequencing signal: handleBurst dispatches a burst's file events and only
		// then arms the git timer, so a dispatched event for this write proves its
		// burst was processed (and armed the debounce).
		_ = d.waitForChange(pathIs(rel))
	}

	assert.Equal(t, 0, git.calls(), "no git recompute may run while the debounce keeps being re-armed")

	timer.Fire()
	<-git.called // the coalesced recompute actually ran

	assert.GreaterOrEqual(t, timer.armCount(), bursts, "every burst must re-arm the trailing debounce")
	assert.Equal(t, 1, git.calls(), "the bursts must coalesce into exactly one git recompute")
}

// The trailing recompute is never dropped: with the production timer in place a
// quiet period after a burst really does drive one git recompute. The test
// blocks on the git provider's own invocation signal — no polling, no window.
func TestWatcher_GitRecompute_TrailingRunFiresOnProductionTimer(
	t *testing.T,
) {
	dir := t.TempDir()
	git := newCountingGit()
	d := newFakeDispatcher()
	w := watch.NewWatcher("ws-trailing", dir, "", git, d)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "trailing.txt"), []byte("x"), 0o600))
	_ = d.waitForChange(pathIs("trailing.txt"))

	<-git.called
	assert.GreaterOrEqual(t, git.calls(), 1, "the trailing git recompute must run once the watcher goes quiet")
}
