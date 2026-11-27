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

type manualGitTimer struct {
	mu   sync.Mutex
	arms int
	c    chan time.Time
}

func newManualGitTimer() *manualGitTimer {
	return &manualGitTimer{c: make(chan time.Time, 1)}
}

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

func (f *fakeGit) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	return gitdomain.DiffBase{}
}

func (f *fakeGit) DiffStat(
	_ context.Context,
	_ string,
	_ string,
) (int, int, error) {
	return 0, 0, nil
}

type fakeDispatcher struct {
	mu      sync.Mutex
	changes []domain.FileChangeEvent
	pulse   chan struct{}
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

func (f *fakeDispatcher) OnGitStatus(context.Context, string, gitdomain.GitStatus) {}

func (f *fakeDispatcher) OnGitStatusUnreadable(context.Context, string, error) {}

func (f *fakeDispatcher) OnSyncWorkingTreeState(context.Context, watch.SyncInput) {}

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
	return w, d
}

func TestWatcher_DetectsFileCreate(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws1")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o600))

	want := domain.FileChangeEvent{Type: domain.FileChangeCreated, Authority: domain.DocumentAuthorityWorkspace, Path: "new.txt", WsID: "ws1"}
	assert.Equal(t, want, d.waitForChange(pathIs("new.txt")))
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

	require.NoError(t, os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("s"), 0o600))
	_ = d.waitForChange(pathIs("sentinel.txt"))

	assert.False(t, d.hasChange(func(evt domain.FileChangeEvent) bool {
		return strings.Contains(evt.Path, "COMMIT_EDITMSG")
	}), "COMMIT_EDITMSG must never surface as a file-change event")
}

func TestWatcher_DispatchesDotGitPrefixedWorkingTreeFiles(
	t *testing.T,
) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o700))
	_, d := newWatcher(t, dir, "ws-dotgit-prefix")

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "COMMIT_EDITMSG"), []byte("msg"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("x"), 0o600))
	workflow := filepath.Join(".github", "workflows", "x.yml")
	require.NoError(t, os.WriteFile(filepath.Join(dir, workflow), []byte("x"), 0o600))

	_ = d.waitForChange(pathIs(".gitignore"))
	_ = d.waitForChange(pathIs(workflow))
	assert.False(t, d.hasChange(func(evt domain.FileChangeEvent) bool {
		return strings.Contains(evt.Path, "COMMIT_EDITMSG")
	}), "a write under .git must still never surface as a file-change event")
}

func TestWatcher_StopIdempotent(
	t *testing.T,
) {
	dir := t.TempDir()
	w := watch.NewWatcher("ws5", dir, "", &fakeGit{}, newFakeDispatcher())
	require.NoError(t, w.Start(context.Background()))
	w.Stop()
	w.Stop()
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

	_ = d.waitForChange(pathIs("subdir"))
}

func TestWatcher_FileInNewNestedDirIsDispatched(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws-nested")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o700))
	_ = d.waitForChange(pathIs("a"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "c.txt"), []byte("x"), 0o600))
	evt := d.waitForChange(pathIs(filepath.Join("a", "b", "c.txt")))
	assert.Equal(t, domain.FileChangeCreated, evt.Type)
}

func TestWatcher_RewriteOfNewFileIsModified(
	t *testing.T,
) {
	dir := t.TempDir()
	_, d := newWatcher(t, dir, "ws-rewrite")
	path := filepath.Join(dir, "fresh.txt")

	require.NoError(t, os.WriteFile(path, []byte("v1"), 0o600))
	_ = d.waitForChange(func(e domain.FileChangeEvent) bool {
		return e.Path == "fresh.txt" && e.Type == domain.FileChangeCreated
	})

	require.NoError(t, os.WriteFile(path, []byte("v2"), 0o600))
	_ = d.waitForChange(func(e domain.FileChangeEvent) bool {
		return e.Path == "fresh.txt" && e.Type == domain.FileChangeModified
	})
}

func underDir(
	rel string,
) func(domain.FileChangeEvent) bool {
	return func(evt domain.FileChangeEvent) bool {
		return strings.HasPrefix(evt.Path, rel+string(filepath.Separator))
	}
}

func TestWatcher_GitignoredDirChangesAreNeverDispatched(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o700))
	_, d := newWatcher(t, dir, "ws-ignored")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "top.js"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("x"), 0o600))

	_ = d.waitForChange(pathIs(filepath.Join("src", "main.go")))
	assert.False(t, d.hasChange(underDir("node_modules")), "a gitignored tree must never be dispatched")
}

func TestWatcher_GitignoredDirCreatedAfterStartIsNotFollowed(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0o600))
	_, d := newWatcher(t, dir, "ws-ignored-later")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o700))
	_ = d.waitForChange(pathIs("dist"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "out.js"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("s"), 0o600))
	_ = d.waitForChange(pathIs("sentinel.txt"))
	assert.False(t, d.hasChange(underDir("dist")), "a gitignored directory created after Start must not be followed")
}

func openDescriptors(
	t *testing.T,
) int {
	t.Helper()
	f, err := os.Open("/dev/fd")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(-1)
	require.NoError(t, err)
	return len(names)
}

func TestWatcher_HeldDescriptorsDoNotGrowWithFileCount(
	t *testing.T,
) {
	const dirs, filesPerDir = 50, 40
	dir := t.TempDir()
	for i := range dirs {
		sub := filepath.Join(dir, fmt.Sprintf("d%02d", i))
		require.NoError(t, os.MkdirAll(sub, 0o700))
		for j := range filesPerDir {
			require.NoError(t, os.WriteFile(filepath.Join(sub, fmt.Sprintf("f%02d.txt", j)), []byte("x"), 0o600))
		}
	}

	before := openDescriptors(t)
	newWatcher(t, dir, "ws-fds")
	held := openDescriptors(t) - before

	t.Logf("descriptors held by a watcher over %d files in %d dirs: %d", dirs*filesPerDir, dirs, held)
	assert.Less(t, held, 16, "a watcher's descriptors must not scale with the files it watches")
}

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

func (g *countingGit) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	return gitdomain.DiffBase{}
}

func (g *countingGit) DiffStat(
	_ context.Context,
	_ string,
	_ string,
) (int, int, error) {
	return 0, 0, nil
}

func (g *countingGit) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.statusCalls
}

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
		_ = d.waitForChange(pathIs(rel))
	}

	assert.Equal(t, 0, git.calls(), "no git recompute may run while the debounce keeps being re-armed")

	timer.Fire()
	<-git.called

	assert.GreaterOrEqual(t, timer.armCount(), bursts, "every burst must re-arm the trailing debounce")
	assert.Equal(t, 1, git.calls(), "the bursts must coalesce into exactly one git recompute")
}

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
