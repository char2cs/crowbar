package realtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
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

// fakeOriginFetcher records every FetchRef call on a channel so tests block on
// the REAL "a fetch happened" signal instead of sleeping.
type fakeOriginFetcher struct {
	calls chan string
	// ffCalls records `merge --ff-only` calls as "<repoPath>:<ref>", buffered like
	// calls so a fetcher never blocks a run goroutine.
	ffCalls chan string
	// ffErr, when set, is what MergeFFOnly returns: git refusing a dirty tree.
	ffErr error
	// hold, when set, keeps every FetchRef in flight until it is closed.
	hold chan struct{}
}

func newFakeOriginFetcher() *fakeOriginFetcher {
	return &fakeOriginFetcher{calls: make(chan string, 64), ffCalls: make(chan string, 64)}
}

func (f *fakeOriginFetcher) MergeFFOnly(
	_ context.Context,
	repoPath string,
	ref string,
) error {
	f.ffCalls <- repoPath + ":" + ref
	return f.ffErr
}

func (f *fakeOriginFetcher) FetchRef(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.calls <- repoPath + ":" + branch
	if f.hold != nil {
		<-f.hold
	}
	return nil
}

// testOriginSyncInterval is far longer than any test runs: cycles happen only
// when a test fires the injected tick source (originDriver), never on a clock,
// because a real origin sync can hold the per-clone git mutex for up to 30 s.
const testOriginSyncInterval = time.Hour

// originDriver owns the manager's two seams. ticks is UNBUFFERED, so a send is a
// rendezvous proving a live runner took the tick; cycles receives once per
// COMPLETED cycle (open-time sync included), which a "fetched nothing" check needs.
type originDriver struct {
	ticks  chan time.Time
	cycles chan struct{}
}

func driveOriginSyncs(
	m *OriginSyncManager,
) *originDriver {
	d := &originDriver{ticks: make(chan time.Time), cycles: make(chan struct{}, 64)}
	m.driveCyclesForTest(d.ticks, d.cycles)
	return d
}

// tick fires exactly one cycle and returns once that cycle has completed.
func (d *originDriver) tick() {
	d.ticks <- time.Time{}
	<-d.cycles
}

func protectedWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
}

// lockedRootWorkspace is a provisioned, LOCKED protected root — the only shape
// the open-time sync is allowed to fast-forward.
func lockedRootWorkspace(
	id string,
) domain.Workspace {
	ws := protectedWorkspace(id)
	ws.Status = domain.WorkspaceStatusLocked
	return ws
}

func childWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "feature", ParentID: "parent-1", Provisioning: domain.WorkspaceProvisioned}
}

func TestOriginSyncManager_Acquire_SyncsProtectedWorkspace(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	assert.Equal(t, "/repo/w1:develop", <-f.calls,
		"FetchRef was not invoked after Acquire for a protected workspace")
	<-d.cycles

	// And the cadence keeps syncing: one tick, one more fetch.
	d.tick()
	assert.Equal(t, "/repo/w1:develop", <-f.calls, "a cadence tick must sync again")
}

func TestOriginSyncManager_Acquire_SkipsWorkspaceWithParent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(childWorkspace("w2"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w2")

	// Block on the cycle COMPLETING, then assert it fetched nothing.
	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef fired for a workspace with a parent")
}

func TestOriginSyncManager_Acquire_SyncsImmediately(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	// The tick source is never fired below, so the fetch observed here CANNOT have
	// come from the cadence — it is the immediate-on-Acquire sync.
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	assert.Equal(t, "/repo/w1:develop", <-f.calls,
		"Acquire did not sync immediately (waited for the interval)")
	<-d.cycles
}

func TestOriginSyncManager_Release_StopsSync(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	<-f.calls  // syncing started
	<-d.cycles // ... and that cycle finished

	m.Release("w1")
	m.waitRunnersForTest() // the run goroutine has RETURNED — not "probably has"

	assert.Empty(t, f.calls, "sync fired after Release stopped the workspace")
}

func TestOriginSyncManager_Refcount(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	m.Acquire("w1")
	<-f.calls
	<-d.cycles

	m.Release("w1") // refs 2 -> 1, still syncing

	// The tick is a rendezvous: it can only be taken by a LIVE run goroutine, so
	// completing it proves the sync outlived the release.
	d.tick()
	assert.Equal(t, "/repo/w1:develop", <-f.calls, "sync stopped despite an outstanding subscriber")
}

func TestOriginSyncManager_BlankWsID_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	m.Acquire("")
	require.NotPanics(t, func() { m.Release("") })
	require.NotPanics(t, func() { m.Release("never-acquired") })

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	// No runner was ever started, so no sync can ever be issued.
	m.waitRunnersForTest()
	assert.Empty(t, f.calls, "blank wsID started a sync")
}

func TestOriginSyncManager_StopAll_Idempotent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)

	m.Acquire("w1")
	<-f.calls
	<-d.cycles

	require.NotPanics(t, m.StopAll)
	require.NotPanics(t, m.StopAll) // second call is safe

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

// StopAll returns only once a fetch already in flight has finished: shutdown
// closes the stores, and a test removes the repo, right after it.
func TestOriginSyncManager_StopAll_WaitsForAFetchInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newFakeOriginWorkspaces()
		w.set(protectedWorkspace("w1"))
		f := newFakeOriginFetcher()
		f.hold = make(chan struct{})
		m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
		m.Acquire("w1")
		<-f.calls

		assert.False(t, stopReturnedEarly(m.StopAll, f.hold),
			"StopAll returned while the open-time fetch was still running")
	})
}

func TestOriginSyncManager_AcquireAfterClose_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	driveOriginSyncs(m)

	m.StopAll()
	m.Acquire("w1")

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	m.waitRunnersForTest()
	assert.Empty(t, f.calls, "Acquire after StopAll started a sync")
}

func TestOriginSyncManager_SyncTick_WorkspaceLookupError_Swallowed(t *testing.T) {
	w := newFakeOriginWorkspaces() // no workspace seeded -> Get errors
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)

	require.NotPanics(t, func() { m.Acquire("missing") })

	// The cycle ran to completion (it did not panic and did not wedge) and issued
	// no fetch.
	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef called despite a workspace lookup error")
}

// TestOriginSyncManager_TickSource_DefaultsToIntervalTicker pins the production
// wiring the seam replaces: with no test source, the cadence is a real ticker.
func TestOriginSyncManager_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, newFakeOriginWorkspaces(), newFakeOriginFetcher())

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
}

// blockingFetcher blocks each FetchRef until its context is cancelled, then
// reports the cause, proving a hung remote cannot wedge the run goroutine.
type blockingFetcher struct {
	released chan error
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{released: make(chan error, 8)}
}

func (b *blockingFetcher) MergeFFOnly(
	_ context.Context,
	_ string,
	_ string,
) error {
	return nil
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

// TestOriginSyncManager_SyncTick_CancelsHungFetch has a timeout as its SUBJECT:
// the injected 10 ms deadline stands in for originSyncTimeout and is never used
// as synchronisation; every wait below blocks on a real signal.
func TestOriginSyncManager_SyncTick_CancelsHungFetch(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, b)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.syncTickWithTimeout(context.Background(), "w1", 10*time.Millisecond, false)
	}()

	assert.ErrorIs(t, <-b.released, context.DeadlineExceeded,
		"hung fetch must be cancelled by the per-sync timeout")
	<-done // syncTick returned once the fetch was cancelled
}

// Release and StopAll cancel a hung fetch through its context instead of
// waiting out originSyncTimeout, so StopAll's join is prompt.
func TestOriginSyncManager_CancelReachesAHungFetch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newFakeOriginWorkspaces()
		w.set(protectedWorkspace("w1"))
		w.set(protectedWorkspace("w2"))
		b := newBlockingFetcher()
		m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, b)
		t.Cleanup(m.StopAll)
		m.Acquire("w1")
		m.Acquire("w2")
		synctest.Wait()

		m.Release("w1")
		require.ErrorIs(t, <-b.released, context.Canceled, "Release must cancel its fetch")
		m.StopAll()
		require.ErrorIs(t, <-b.released, context.Canceled, "StopAll must cancel the fetch it joins")
	})
}

// ensure the fakes satisfy their interfaces at compile time.
var (
	_ OriginSyncWorkspaces = (*fakeOriginWorkspaces)(nil)
	_ OriginFetcher        = (*fakeOriginFetcher)(nil)
	_ OriginFetcher        = (*blockingFetcher)(nil)
)

// TestOriginSyncManager_OpenFastForwardsLockedRoot pins the open-time advance:
// opening a locked root fast-forwards it onto origin's ref. It is the ONLY moment
// its working tree moves unasked; every tick's FetchRef touches only the ref.
func TestOriginSyncManager_OpenFastForwardsLockedRoot(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	m.Acquire("w1")
	t.Cleanup(func() { m.Release("w1"); m.waitRunnersForTest() })

	assert.Equal(t, "/repo/w1:develop", <-g.calls, "the open sync refreshes origin/<branch> first")
	assert.Equal(t, "/repo/w1:origin/develop", <-g.ffCalls,
		"then fast-forwards the worktree onto ORIGIN's ref, not the local one")
	<-d.cycles
}

// TestOriginSyncManager_IntervalTickDoesNotFastForward: the sync ticks only while
// the user is looking, so advancing on a tick would move files under a live
// editor or agent. A tick refreshes the ref and stops there.
func TestOriginSyncManager_IntervalTickDoesNotFastForward(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	m.Acquire("w1")
	t.Cleanup(func() { m.Release("w1"); m.waitRunnersForTest() })

	// Drain the open sync (which legitimately advances).
	<-g.calls
	<-g.ffCalls
	<-d.cycles

	// Now a real interval tick: it must fetch, and must NOT advance.
	d.ticks <- time.Now()
	assert.Equal(t, "/repo/w1:develop", <-g.calls, "the tick still refreshes the origin ref")
	<-d.cycles // the cycle RAN to completion...
	assert.Empty(t, g.ffCalls, "...and it fast-forwarded nothing")
}

// TestOriginSyncManager_OpenSkipsWorkspacesItDoesNotOwn: only provisioned, locked,
// non-default roots advance — not the user's repo home, a placeholder with no
// worktree, or an unlocked root that may carry local commits. All still fetch.
func TestOriginSyncManager_OpenSkipsWorkspacesItDoesNotOwn(t *testing.T) {
	home := lockedRootWorkspace("home")
	home.IsDefault = true
	home.Provisioning = domain.WorkspaceShared
	placeholder := lockedRootWorkspace("ph")
	placeholder.WorktreePath = ""
	placeholder.Provisioning = domain.WorkspacePlaceholder
	unlocked := protectedWorkspace("unlocked") // no locked status

	for _, ws := range []domain.Workspace{home, placeholder, unlocked} {
		t.Run(ws.ID, func(t *testing.T) {
			w := newFakeOriginWorkspaces()
			w.set(ws)
			g := newFakeOriginFetcher()
			m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, g)
			d := driveOriginSyncs(m)

			m.Acquire(ws.ID)
			t.Cleanup(func() { m.Release(ws.ID); m.waitRunnersForTest() })

			if ws.WorktreePath != "" {
				assert.Equal(t, ws.WorktreePath+":develop", <-g.calls, "the ref is still refreshed")
			} else {
				<-g.calls // a placeholder still fetches; only the advance is skipped
			}
			<-d.cycles
			assert.Empty(t, g.ffCalls, "this workspace must never be fast-forwarded")
		})
	}
}

// TestOriginSyncManager_OpenSurvivesRefusedFastForward: git refuses rather than
// overwrite local changes, and that refusal must never break opening a workspace.
func TestOriginSyncManager_OpenSurvivesRefusedFastForward(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	g.ffErr = enginegit.ErrDirtyTree
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	m.Acquire("w1")
	t.Cleanup(func() { m.Release("w1"); m.waitRunnersForTest() })

	<-g.calls
	<-g.ffCalls
	<-d.cycles // the cycle completed despite the refusal
}
