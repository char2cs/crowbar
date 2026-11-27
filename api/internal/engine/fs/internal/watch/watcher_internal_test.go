package watch

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
)

type minimalGit struct{}

func (g *minimalGit) ComputeStatus(_ context.Context, _ string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *minimalGit) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	return gitdomain.DiffBase{}
}

func (g *minimalGit) DiffStat(
	_ context.Context,
	_ string,
	_ string,
) (int, int, error) {
	return 0, 0, nil
}

type recordingDispatcher struct {
	mu         sync.Mutex
	gitCount   int
	errCount   int
	syncs      []SyncInput
	files      chan string
	statuses   chan gitdomain.GitStatus
	unreadable chan error
}

func newRecordingDispatcher() *recordingDispatcher {
	return &recordingDispatcher{files: make(chan string, 64)}
}

func (r *recordingDispatcher) OnFileChange(_ context.Context, evt domain.FileChangeEvent) {
	select {
	case r.files <- evt.Path:
	default:
	}
}

func (r *recordingDispatcher) waitForFile(rel string) {
	for path := range r.files {
		if path == rel {
			return
		}
	}
}

func (r *recordingDispatcher) OnGitStatus(_ context.Context, _ string, status gitdomain.GitStatus) {
	r.mu.Lock()
	r.gitCount++
	r.mu.Unlock()
	if r.statuses != nil {
		r.statuses <- status
	}
}

func (r *recordingDispatcher) OnGitStatusUnreadable(_ context.Context, _ string, err error) {
	r.mu.Lock()
	r.errCount++
	r.mu.Unlock()
	if r.unreadable != nil {
		r.unreadable <- err
	}
}

func (r *recordingDispatcher) errors() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errCount
}

func (r *recordingDispatcher) OnSyncWorkingTreeState(_ context.Context, in SyncInput) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncs = append(r.syncs, in)
}

func (r *recordingDispatcher) lastSync() (SyncInput, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.syncs) == 0 {
		return SyncInput{}, false
	}
	return r.syncs[len(r.syncs)-1], true
}

func (r *recordingDispatcher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gitCount
}

func newBareWatcher(repoPath string, d Dispatcher) *Watcher {
	return NewWatcher("ws-internal", repoPath, "", &minimalGit{}, d)
}

func gitRecomputePending(w *Watcher) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gitPending
}

func TestRelPath_NormalCase(t *testing.T) {
	dir := t.TempDir()
	w := newBareWatcher(dir, &recordingDispatcher{})

	absPath := filepath.Join(dir, "sub", "file.txt")
	result := w.relPath(absPath)

	require.Equal(t, filepath.Join("sub", "file.txt"), result)
}

func TestShouldIgnore_DotGitContent(t *testing.T) {
	dir := t.TempDir()
	w := newBareWatcher(dir, &recordingDispatcher{})

	cases := []struct {
		name    string
		path    string
		ignored bool
	}{
		{
			name:    "COMMIT_EDITMSG ignored",
			path:    filepath.Join(dir, ".git", "COMMIT_EDITMSG"),
			ignored: true,
		},
		{
			name:    "MERGE_MSG ignored",
			path:    filepath.Join(dir, ".git", "MERGE_MSG"),
			ignored: true,
		},
		{
			name:    "HEAD not ignored",
			path:    filepath.Join(dir, ".git", "HEAD"),
			ignored: false,
		},
		{
			name:    "refs/heads/main not ignored",
			path:    filepath.Join(dir, ".git", "refs", "heads", "main"),
			ignored: false,
		},
		{
			name:    "packed-refs not ignored",
			path:    filepath.Join(dir, ".git", "packed-refs"),
			ignored: false,
		},
		{
			name:    "regular file not ignored",
			path:    filepath.Join(dir, "main.go"),
			ignored: false,
		},
		{
			name:    "nested regular file not ignored",
			path:    filepath.Join(dir, "pkg", "foo.go"),
			ignored: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.ignored, w.shouldIgnore(tc.path))
		})
	}
}

func TestClassifyChange_AllOps(t *testing.T) {
	cases := []struct {
		op   op
		want domain.FileChangeType
	}{
		{opCreate, domain.FileChangeCreated},
		{opRemove, domain.FileChangeDeleted},
		{opRename, domain.FileChangeRenamed},
		{opWrite, domain.FileChangeModified},
		{opChmod, domain.FileChangeModified},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, classifyChange(tc.op))
	}
}

func TestClassifyChange_CreateBeatsRemove(t *testing.T) {
	assert.Equal(t, domain.FileChangeCreated, classifyChange(opCreate|opRemove))
}

func TestRelPath_EmptyRepoPathFallsBack(t *testing.T) {
	w := newBareWatcher("", &recordingDispatcher{})
	absPath := "/some/absolute/path.go"
	result := w.relPath(absPath)
	assert.Equal(t, absPath, result, "should return raw absPath when filepath.Rel errors")
}

func TestHandleOne_ShouldIgnoreCallsMaybeHandleGitRef(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := newBareWatcher(dir, rd)

	evt := event{
		Name: filepath.Join(dir, ".git", "COMMIT_EDITMSG"),
		Op:   opWrite,
	}
	w.handleOne(context.Background(), evt)

	assert.Equal(t, 0, rd.count())
}

func TestStart_AddRecursiveErrorPropagated(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-start", dir, "", &minimalGit{}, rd)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
}

func requireBackendClosed(
	t *testing.T,
	w *Watcher,
) {
	t.Helper()
	w.mu.Lock()
	be := w.be
	w.mu.Unlock()
	require.NotNil(t, be, "Start must have created a backend")
	for range be.events() {
	}
}

func TestStart_AlreadyCancelledCtx_NoAllocation(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher("ws-cancelled", dir, "", &minimalGit{}, &recordingDispatcher{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.Start(ctx)
	require.ErrorIs(t, err, context.Canceled)

	w.mu.Lock()
	be := w.be
	w.mu.Unlock()
	assert.Nil(t, be, "no backend should be created for a cancelled ctx")
}

func TestStart_StopBeforeStart_ClosesCreatedWatcher(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher("ws-stop-first", dir, "", &minimalGit{}, &recordingDispatcher{})

	w.Stop()

	err := w.Start(context.Background())
	require.ErrorIs(t, err, context.Canceled)

	w.mu.Lock()
	be := w.be
	w.mu.Unlock()
	assert.Nil(t, be, "stopped Start must not register the backend it closed")
}

func TestStartThenStop_ClosesWatcherAndDrainsGoroutine(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-flap", dir, "", &minimalGit{}, rd)

	require.NoError(t, w.Start(context.Background()))

	w.Stop()
	w.Stop()

	<-w.loopDone
	requireBackendClosed(t, w)
}

func TestStart_SecondStartRejected_NoLeak(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher("ws-restart", dir, "", &minimalGit{}, &recordingDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)

	require.NoError(t, w.Start(ctx))

	w.mu.Lock()
	first := w.be
	w.mu.Unlock()

	err := w.Start(ctx)
	require.ErrorIs(t, err, ErrAlreadyStarted)

	w.mu.Lock()
	second := w.be
	w.mu.Unlock()

	assert.Same(t, first, second, "second Start must not overwrite the first backend")

	w.Stop()
	<-w.loopDone
}

func TestStart_AfterStop_Rejected(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher("ws-restart-stopped", dir, "", &minimalGit{}, &recordingDispatcher{})
	require.NoError(t, w.Start(context.Background()))
	w.Stop()

	err := w.Start(context.Background())
	require.ErrorIs(t, err, ErrAlreadyStarted)
}

func TestLoop_ClosedEventsChannelExits(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-loop", dir, "", &minimalGit{}, rd)
	ctx := context.Background()
	require.NoError(t, w.Start(ctx))

	w.mu.Lock()
	_ = w.be.close()
	w.mu.Unlock()

	<-w.loopDone
	w.Stop()
}

func TestAddRecursive_WalkErrCallbackReturnsNil(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o700))

	rd := newRecordingDispatcher()
	w := NewWatcher("ws-walk", dir, "", &minimalGit{}, rd)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))

	require.NoError(t, os.Remove(subdir))

	newSub := filepath.Join(dir, "newsub")
	require.NoError(t, os.MkdirAll(newSub, 0o700))

	rd.waitForFile("newsub")
}

func TestAddRecursive_PermissionDeniedSwallowed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks are bypassed")
	}
	dir := t.TempDir()
	noPermDir := filepath.Join(dir, "noperm")
	require.NoError(t, os.MkdirAll(noPermDir, 0o700))
	require.NoError(t, os.Chmod(noPermDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(noPermDir, 0o700) })

	w := NewWatcher("ws-perm", dir, "", &minimalGit{}, &recordingDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)

	require.NoError(t, w.Start(ctx))
}

func TestStart_AddRecursiveErrorClosesFSW(t *testing.T) {
	dir := t.TempDir()
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-adderr", dir, "", &minimalGit{}, rd)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
}

func TestLoop_ErrorsChannelSwallowed(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher("ws-err-loop", dir, "", &minimalGit{}, &recordingDispatcher{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(w.Stop)
	require.NoError(t, w.Start(ctx))
}

func TestLoop_TimerDrainPath(t *testing.T) {
	dir := t.TempDir()
	rd := newRecordingDispatcher()
	w := NewWatcher("ws-timer", dir, "", &minimalGit{}, rd)

	be, err := newBackend(t.Context(), w.spec())
	require.NoError(t, err)
	w.mu.Lock()
	w.be = be
	w.mu.Unlock()
	t.Cleanup(func() { _ = be.close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.loop(ctx)
	}()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "loop_test.txt"), []byte("data"), 0o600))

	rd.waitForFile("loop_test.txt")

	cancel()
	<-done
}
