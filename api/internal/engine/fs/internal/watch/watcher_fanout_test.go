package watch_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// captureDispatcher records every callback. File-change waits block on a pulse
// (see fakeDispatcher); git/sync results are read after the recompute barrier
// in capturingWatcher.recomputeNow, which is itself a real signal from the loop
// goroutine — so nothing here needs a poll interval.
type captureDispatcher struct {
	mu        sync.Mutex
	gitCalls  int
	syncCalls []watch.SyncInput
	fileCalls []domain.FileChangeEvent
	pulse     chan struct{}
}

func newCaptureDispatcher() *captureDispatcher {
	return &captureDispatcher{pulse: make(chan struct{}, 1)}
}

func (c *captureDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	c.mu.Lock()
	c.fileCalls = append(c.fileCalls, evt)
	c.mu.Unlock()
	select {
	case c.pulse <- struct{}{}:
	default:
	}
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

// waitForFileChange blocks until a dispatched event satisfies match.
func (c *captureDispatcher) waitForFileChange(
	match func(domain.FileChangeEvent) bool,
) domain.FileChangeEvent {
	for {
		c.mu.Lock()
		for _, evt := range c.fileCalls {
			if match(evt) {
				c.mu.Unlock()
				return evt
			}
		}
		c.mu.Unlock()
		<-c.pulse
	}
}

func (c *captureDispatcher) syncs() []watch.SyncInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]watch.SyncInput(nil), c.syncCalls...)
}

func (c *captureDispatcher) gitStatusCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gitCalls
}

// capturingWatcher is a started Watcher whose trailing git-recompute debounce is
// fired by the test, plus the loop's own "recompute complete" signal. Together
// they turn every fanOutGit assertion below into a statement about a recompute
// that provably ran and provably finished — including the ones asserting that
// *no* sync was dispatched, which previously could only hope that nothing would
// show up inside a 400ms window.
type capturingWatcher struct {
	w          *watch.Watcher
	d          *captureDispatcher
	timer      *manualGitTimer
	recomputed chan struct{}
}

func newCapturingWatcher(
	t *testing.T,
	dir string,
	git watch.GitStatusProvider,
) *capturingWatcher {
	t.Helper()
	d := newCaptureDispatcher()
	w := watch.NewWatcher("ws-cap", dir, "", git, d)

	timer := newManualGitTimer()
	recomputed := make(chan struct{}, 16)
	w.SetGitTimerForTest(timer)
	w.SetOnGitRecomputeForTest(func() {
		select {
		case recomputed <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	return &capturingWatcher{w: w, d: d, timer: timer, recomputed: recomputed}
}

// recomputeNow releases the debounce and blocks until the loop goroutine reports
// that the git recompute has completed. On return, every effect that recompute
// was going to have (OnGitStatus, OnSyncWorkingTreeState — or the deliberate
// absence of them) has already been dispatched.
func (c *capturingWatcher) recomputeNow() {
	c.timer.Fire()
	<-c.recomputed
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
	cw := newCapturingWatcher(t, dir, git)

	// Recompute #1 returns 0,0,false,false — identical to the zero-value prev
	// state, so no sync is dispatched.
	cw.recomputeNow()
	assert.Empty(t, cw.d.syncs(), "an unchanged summary must not dispatch a sync")

	// Recompute #2: changingGit now returns 1,0,false,false → sync.
	cw.recomputeNow()

	syncs := cw.d.syncs()
	require.Len(t, syncs, 1)
	assert.Equal(t, 1, syncs[0].Added)
	assert.Equal(t, "ws-cap", syncs[0].WsID)
}

// ---------------------------------------------------------------------------
// fanOutGit — MERGE_HEAD suppresses OnSyncWorkingTreeState
// ---------------------------------------------------------------------------

func TestWatcher_FanOutGit_MergeHeadSuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(gitDir, "MERGE_HEAD"),
		[]byte("deadbeef\n"),
		0o600,
	))

	cw := newCapturingWatcher(t, dir, git)

	// Two recomputes that both provably ran and completed: the rewrite guard must
	// short-circuit each one before it ever reaches git.
	cw.recomputeNow()
	cw.recomputeNow()

	assert.Empty(t, cw.d.syncs(), "a mid-merge recompute must not dispatch a sync")
	assert.Zero(t, git.summaryCalls(), "a mid-merge recompute must not even shell out to git")
	assert.Zero(t, cw.d.gitStatusCalls(), "a mid-merge recompute must not broadcast git status")
}

// ---------------------------------------------------------------------------
// relPath — normal path returns relative value
// ---------------------------------------------------------------------------

func TestWatcher_RelPath_NormalPathIsRelative(t *testing.T) {
	dir := t.TempDir()
	cw := newCapturingWatcher(t, dir, &fakeGit{})

	filePath := filepath.Join(dir, "rel_test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("data"), 0o600))

	evt := cw.d.waitForFileChange(pathIs("rel_test.txt"))
	assert.Equal(t, "rel_test.txt", evt.Path)
}

// ---------------------------------------------------------------------------
// classifyChange — Rename op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Rename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.txt")
	dst := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))

	cw := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.Rename(src, dst))

	evt := cw.d.waitForFileChange(func(e domain.FileChangeEvent) bool {
		return e.Type == domain.FileChangeRenamed
	})
	assert.Equal(t, domain.FileChangeRenamed, evt.Type)
}

// ---------------------------------------------------------------------------
// classifyChange — Modify op
// ---------------------------------------------------------------------------

func TestWatcher_ClassifyChange_Modify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mod.txt")
	require.NoError(t, os.WriteFile(path, []byte("v1"), 0o600))

	cw := newCapturingWatcher(t, dir, &fakeGit{})

	require.NoError(t, os.WriteFile(path, []byte("v2"), 0o600))

	evt := cw.d.waitForFileChange(func(e domain.FileChangeEvent) bool {
		return e.Path == "mod.txt" && e.Type == domain.FileChangeModified
	})
	assert.Equal(t, domain.FileChangeModified, evt.Type)
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
	cw := newCapturingWatcher(t, dir, &commitTogglingGit{})

	cw.recomputeNow() // prime: no toggle yet
	cw.recomputeNow() // hasCommits flips false → true

	syncs := cw.d.syncs()
	require.Len(t, syncs, 1)
	assert.True(t, syncs[0].HasCommits)
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
	cw := newCapturingWatcher(t, dir, &conflictTogglingGit{})

	cw.recomputeNow() // prime: no conflicts yet
	cw.recomputeNow() // hasConflicts flips false → true

	syncs := cw.d.syncs()
	require.Len(t, syncs, 1)
	assert.True(t, syncs[0].HasConflicts)
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
	cw := newCapturingWatcher(t, dir, &deletedChangeGit{})

	cw.recomputeNow() // prime: no change yet
	cw.recomputeNow() // deleted count 0 → 3

	syncs := cw.d.syncs()
	require.Len(t, syncs, 1)
	assert.Equal(t, 3, syncs[0].Deleted)
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
	cw := newCapturingWatcher(t, dir, &summaryErrorGit{})

	cw.recomputeNow()

	assert.Equal(t, 1, cw.d.gitStatusCalls(), "the status half of the recompute still broadcasts")
	assert.Empty(t, cw.d.syncs(), "a failed summary must not dispatch a sync")
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
	cw := newCapturingWatcher(t, dir, git)

	cw.recomputeNow()

	// The recompute provably ran (ComputeStatus was attempted) and provably
	// finished, so the absence of a broadcast is an assertion, not a guess.
	assert.Equal(t, 1, git.statusCalls(), "the recompute must attempt ComputeStatus")
	assert.Zero(t, cw.d.gitStatusCalls(), "OnGitStatus must not be called when ComputeStatus fails")
	assert.Empty(t, cw.d.syncs(), "a failed status must not dispatch a sync")
}

// ---------------------------------------------------------------------------
// isRewriteInProgress — rebase-merge and rebase-apply also suppress sync
// ---------------------------------------------------------------------------

func TestWatcher_FanOutGit_RebaseMergeSuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0o700))

	cw := newCapturingWatcher(t, dir, git)

	cw.recomputeNow()
	cw.recomputeNow()

	assert.Empty(t, cw.d.syncs(), "a mid-rebase recompute must not dispatch a sync")
	assert.Zero(t, git.summaryCalls(), "a mid-rebase recompute must not shell out to git")
}

func TestWatcher_FanOutGit_RebaseApplySuppressesSync(t *testing.T) {
	dir := t.TempDir()
	git := &changingGit{}

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "rebase-apply"), 0o700))

	cw := newCapturingWatcher(t, dir, git)

	cw.recomputeNow()
	cw.recomputeNow()

	assert.Empty(t, cw.d.syncs(), "a mid-rebase recompute must not dispatch a sync")
	assert.Zero(t, git.summaryCalls(), "a mid-rebase recompute must not shell out to git")
}
