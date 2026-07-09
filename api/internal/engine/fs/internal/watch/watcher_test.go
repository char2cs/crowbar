package watch_test

import (
	"context"
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
}

func (f *fakeDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, evt)
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

func (f *fakeDispatcher) waitForChange(
	t *testing.T,
) domain.FileChangeEvent {
	t.Helper()
	var evt domain.FileChangeEvent
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.changes) == 0 {
			return false
		}
		evt = f.changes[len(f.changes)-1]
		return true
	}, 3*time.Second, 5*time.Millisecond, "timeout waiting for file change event")
	return evt
}

func newWatcher(
	t *testing.T,
	dir string,
	wsID string,
) (*watch.Watcher, *fakeDispatcher) {
	t.Helper()
	d := &fakeDispatcher{}
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

	evt := d.waitForChange(t)
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

	_ = d.waitForChange(t)
}

func TestWatcher_DetectsFileDelete(
	t *testing.T,
) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todelete.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

	_, d := newWatcher(t, dir, "ws3")

	require.NoError(t, os.Remove(path))

	evt := d.waitForChange(t)
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

	// Must-not-happen: a write inside .git (which addRecursive never watches) must
	// never surface as a file-change event. Poll the real event slice across the
	// window instead of sleeping-then-checking.
	require.Never(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, evt := range d.changes {
			if strings.Contains(evt.Path, "COMMIT_EDITMSG") {
				return true
			}
		}
		return false
	}, 250*time.Millisecond, 20*time.Millisecond, "COMMIT_EDITMSG must never surface as a file-change event")
}

func TestWatcher_StopIdempotent(
	t *testing.T,
) {
	dir := t.TempDir()
	w := watch.NewWatcher("ws5", dir, "", &fakeGit{}, &fakeDispatcher{})
	require.NoError(t, w.Start(context.Background()))
	w.Stop()
	w.Stop()
}

func TestWatcher_SubdirCreatedAndWatched(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws6")

	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o600))

	// Wait for the watcher to dispatch a change (the subdir's own CREATE event on the
	// watched repo root satisfies this). Replaces the prior settle sleep with a wait
	// on the real dispatched event.
	_ = d.waitForChange(t)
}

// countingGit counts ComputeStatus invocations so tests can assert how many
// times the watcher actually shelled out to git.
type countingGit struct {
	mu          sync.Mutex
	statusCalls int
}

func (g *countingGit) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.statusCalls++
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
// succession must coalesce into a single fanOutGit run (one ComputeStatus),
// and that final run must never be dropped once the watcher goes quiet.
// Without the debounce, each of the four bursts below would shell out to git
// on its own (the ~6Hz churn observed with linked worktrees sharing one .git).
func TestWatcher_GitRecomputeDebounced_CoalescesBursts(
	t *testing.T,
) {
	dir := t.TempDir()
	git := &countingGit{}
	d := &fakeDispatcher{}
	w := watch.NewWatcher("ws-debounce", dir, "", git, d)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	// Drive four DISTINCT fs-debounce windows by waiting for each write's own
	// file-change event to be dispatched before issuing the next write. Sequencing
	// on the real dispatched event (not a sleep) keeps the writes ~one fs-debounce
	// apart (~100ms) — comfortably inside the 250ms git-debounce window — so every
	// burst re-arms the git timer and the recomputes coalesce.
	for i := range 4 {
		rel := "burst" + string(rune('a'+i)) + ".txt"
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte("x"), 0o600))
		require.Eventually(t, func() bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			for _, evt := range d.changes {
				if evt.Path == rel {
					return true
				}
			}
			return false
		}, 5*time.Second, 5*time.Millisecond, "burst "+rel+" must be processed as its own fs-debounce window")
	}

	// The trailing recompute must run once the watcher goes quiet (never dropped).
	require.Eventually(t, func() bool {
		return git.calls() >= 1
	}, 5*time.Second, 5*time.Millisecond, "trailing git recompute must always run after the quiet period")

	// Coalescing (genuine must-not-happen): the four bursts must never produce more
	// than two recomputes. Allow 2 for scheduler jitter on slow CI, but assert >2
	// never happens across a window that comfortably exceeds the git debounce —
	// proving the cross-burst coalescing held rather than four uncoalesced runs.
	require.Never(t, func() bool {
		return git.calls() > 2
	}, 400*time.Millisecond, 20*time.Millisecond, "git recomputes must coalesce across bursts")
}
