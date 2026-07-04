package realtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeOriginWorkspaces is an in-memory OriginSyncWorkspaces fake keyed by
// workspace ID, safe for concurrent Get/set from the manager's goroutines and
// the test's main goroutine.
type fakeOriginWorkspaces struct {
	mu   sync.Mutex
	byID map[string]domain.Workspace
}

func newFakeOriginWorkspaces() *fakeOriginWorkspaces {
	return &fakeOriginWorkspaces{byID: make(map[string]domain.Workspace)}
}

func (f *fakeOriginWorkspaces) set(
	ws domain.Workspace,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[ws.ID] = ws
}

func (f *fakeOriginWorkspaces) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ws, ok := f.byID[id]
	if !ok {
		return domain.Workspace{}, fmt.Errorf("workspace %s not found", id)
	}
	return ws, nil
}

// fakeOriginFetcher records every FetchRef call on a channel so tests can
// synchronise on the per-connection ticker without sleeping.
type fakeOriginFetcher struct {
	calls chan string
}

func newFakeOriginFetcher() *fakeOriginFetcher {
	return &fakeOriginFetcher{calls: make(chan string, 64)}
}

func (f *fakeOriginFetcher) FetchRef(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.calls <- repoPath + ":" + branch
	return nil
}

const testOriginSyncInterval = time.Millisecond

func protectedWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "develop"}
}

func childWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "feature", ParentID: "parent-1"}
}

func TestOriginSyncManager_Acquire_SyncsProtectedWorkspace(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("FetchRef was not invoked after Acquire for a protected workspace")
	}
}

func TestOriginSyncManager_Acquire_SkipsWorkspaceWithParent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(childWorkspace("w2"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w2")

	select {
	case <-f.calls:
		t.Fatal("FetchRef fired for a workspace with a parent")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_Acquire_SyncsImmediately(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	// A long interval ensures the ticker cannot fire within the test window, so
	// observing a fetch proves the immediate-on-Acquire sync, not a ticker tick.
	m := NewOriginSyncManager(context.Background(), time.Hour, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("Acquire did not sync immediately (waited for the interval)")
	}
}

func TestOriginSyncManager_Release_StopsSync(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	<-f.calls // confirm syncing started
	m.Release("w1")

	drainOriginCalls(f.calls)

	select {
	case <-f.calls:
		t.Fatal("sync fired after Release stopped the workspace")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_Refcount(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	m.Acquire("w1")
	<-f.calls
	m.Release("w1") // refs 2 -> 1, still syncing

	drainOriginCalls(f.calls)

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("sync stopped despite an outstanding subscriber")
	}
}

func TestOriginSyncManager_BlankWsID_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("")
	require.NotPanics(t, func() { m.Release("") })
	require.NotPanics(t, func() { m.Release("never-acquired") })

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-f.calls:
		t.Fatal("blank wsID started a sync")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_StopAll_Idempotent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)

	m.Acquire("w1")
	<-f.calls

	require.NotPanics(t, m.StopAll)
	require.NotPanics(t, m.StopAll) // second call is safe

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestOriginSyncManager_AcquireAfterClose_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)

	m.StopAll()
	m.Acquire("w1")

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-f.calls:
		t.Fatal("Acquire after StopAll started a sync")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_SyncTick_WorkspaceLookupError_Swallowed(t *testing.T) {
	w := newFakeOriginWorkspaces() // no workspace seeded -> Get errors
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	require.NotPanics(t, func() { m.Acquire("missing") })

	select {
	case <-f.calls:
		t.Fatal("FetchRef called despite a workspace lookup error")
	case <-time.After(20 * time.Millisecond):
	}
}

// blockingFetcher blocks each FetchRef on the per-fetch context until that
// context is cancelled, then reports the cancellation cause on a channel. It
// proves the manager bounds a hung fetch: a remote that never returns must not
// wedge the run goroutine forever.
type blockingFetcher struct {
	released chan error
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{released: make(chan error, 8)}
}

func (b *blockingFetcher) FetchRef(
	ctx context.Context,
	_ string,
	_ string,
) error {
	<-ctx.Done()
	b.released <- ctx.Err()
	return ctx.Err()
}

func TestOriginSyncManager_SyncTick_CancelsHungFetch(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), time.Hour, w, b)

	// Drive a single tick directly with a short injected timeout so the test
	// does not wait the production 30s. The fetch blocks on ctx.Done(); the
	// timeout must fire and unblock it, proving a hung remote cannot wedge the
	// run goroutine.
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		m.syncTickWithTimeout(ctx, "w1", 10*time.Millisecond)
		close(done)
	}()

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"hung fetch must be cancelled by the per-sync timeout")
	case <-time.After(time.Second):
		t.Fatal("fetch was never cancelled — the per-sync timeout did not fire")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("syncTick did not return after the fetch was cancelled")
	}
}

func TestOriginSyncManager_SyncTick_DecoupledFromConnCtx(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), time.Hour, w, b)

	// The per-connection ctx is ALREADY cancelled, yet WithoutCancel must let
	// the fetch start (it only stops via its own timeout). This preserves the
	// best-effort background-sync guarantee while still bounding the fetch.
	connCtx, cancelConn := context.WithCancel(context.Background())
	cancelConn()

	go m.syncTickWithTimeout(connCtx, "w1", 10*time.Millisecond)

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"fetch must run under WithoutCancel and stop only on its own timeout")
	case <-time.After(time.Second):
		t.Fatal("fetch did not start/cancel despite an already-cancelled conn ctx")
	}
}

// drainOriginCalls empties any already-buffered calls so a subsequent receive
// observes only post-drain activity.
func drainOriginCalls(
	c chan string,
) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// ensure the fakes satisfy their interfaces at compile time.
var (
	_ OriginSyncWorkspaces = (*fakeOriginWorkspaces)(nil)
	_ OriginFetcher        = (*fakeOriginFetcher)(nil)
	_ OriginFetcher        = (*blockingFetcher)(nil)
)
