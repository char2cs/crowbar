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
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.changes)
		f.mu.Unlock()
		if n > 0 {
			f.mu.Lock()
			evt := f.changes[len(f.changes)-1]
			f.mu.Unlock()
			return evt
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for file change event")
	return domain.FileChangeEvent{}
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
	time.Sleep(50 * time.Millisecond)
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

	time.Sleep(250 * time.Millisecond)

	d.mu.Lock()
	for _, evt := range d.changes {
		assert.NotContains(t, evt.Path, "COMMIT_EDITMSG")
	}
	d.mu.Unlock()
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
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0o600))

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
	time.Sleep(50 * time.Millisecond)

	// Four bursts spaced wider than the fs debounce (100ms) but inside the
	// git debounce window (250ms), so each burst re-arms the git timer.
	for i := range 4 {
		name := filepath.Join(dir, "burst"+string(rune('a'+i))+".txt")
		require.NoError(t, os.WriteFile(name, []byte("x"), 0o600))
		time.Sleep(150 * time.Millisecond)
	}

	// Quiet period: the trailing recompute must run (never dropped).
	deadline := time.Now().Add(3 * time.Second)
	for git.calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	require.GreaterOrEqual(t, git.calls(), 1, "trailing git recompute must always run after the quiet period")

	// Let everything settle, then assert the bursts coalesced. Allow 2 for
	// scheduler jitter on slow CI, but four uncoalesced runs must not happen.
	time.Sleep(400 * time.Millisecond)
	assert.LessOrEqual(t, git.calls(), 2, "git recomputes must coalesce across bursts")
}
