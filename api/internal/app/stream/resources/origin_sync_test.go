package resources

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

// fakeOriginWorkspaces is an in-memory OriginSyncWorkspaces keyed by workspace
// ID, safe for the manager's goroutines and the test's at once.
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
	// ffCalls records `merge --ff-only` calls as "<repoPath>:<ref>", like calls.
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
// when a test fires the injected tick source (originDriver), never on a clock.
const testOriginSyncInterval = time.Hour

// originDriver owns the manager's two seams. ticks is UNBUFFERED, so a send is a
// rendezvous proving a live runner took the tick; cycles receives once per
// COMPLETED cycle (open-time sync included), which a "fetched nothing" check needs.
type originDriver struct {
	ticks  chan time.Time
	cycles chan struct{}
}

func driveOriginSyncs(
	m *OriginSync,
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

func TestOriginSync_Start_SyncsProtectedWorkspace(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))

	assert.Equal(t, "/repo/w1:develop", <-f.calls,
		"FetchRef was not invoked after Start for a protected workspace")
	<-d.cycles

	// And the cadence keeps syncing: one tick, one more fetch.
	d.tick()
	assert.Equal(t, "/repo/w1:develop", <-f.calls, "a cadence tick must sync again")
}

func TestOriginSync_Start_SkipsWorkspaceWithParent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(childWorkspace("w2"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w2"))

	// Block on the cycle COMPLETING, then assert it fetched nothing.
	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef fired for a workspace with a parent")
}

func TestOriginSync_Start_SyncsImmediately(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	// The tick source is never fired, so this fetch is the immediate-on-Start one.
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))

	assert.Equal(t, "/repo/w1:develop", <-f.calls,
		"Start did not sync immediately (waited for the interval)")
	<-d.cycles
}

func TestOriginSync_Stop_StopsSync(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NoError(t, m.Start("w1"))
	<-f.calls  // syncing started
	<-d.cycles // ... and that cycle finished

	m.Stop("w1")
	m.waitRunnersForTest() // the run goroutine has RETURNED — not "probably has"

	assert.Empty(t, f.calls, "sync fired after Stop stopped the workspace")
}

func TestOriginSync_Close_Idempotent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)

	require.NoError(t, m.Start("w1"))
	<-f.calls
	<-d.cycles

	require.NotPanics(t, m.Close)
	require.NotPanics(t, m.Close) // second call is safe

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

// Close returns only once a fetch already in flight has finished: shutdown
// closes the stores, and a test removes the repo, right after it.
func TestOriginSync_Close_WaitsForAFetchInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newFakeOriginWorkspaces()
		w.set(protectedWorkspace("w1"))
		f := newFakeOriginFetcher()
		f.hold = make(chan struct{})
		m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
		require.NoError(t, m.Start("w1"))
		<-f.calls

		assert.False(t, stopReturnedEarly(m.Close, f.hold),
			"Close returned while the open-time fetch was still running")
	})
}

// A cancel decides whether the fast-forward starts: a fetch that finishes after
// Stop must not go on to move the tree.
func TestOriginSync_CancelBeforeTheFastForwardSkipsIt(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	g.hold = make(chan struct{})
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	require.NoError(t, m.Start("w1"))
	<-g.calls // the open-time fetch is in flight

	m.Stop("w1")
	close(g.hold)
	m.waitRunnersForTest()
	assert.Empty(t, g.ffCalls, "the fast-forward started after its cancel")
}

func TestOriginSync_StartAfterClose_Refused(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	driveOriginSyncs(m)

	m.Close()
	require.ErrorIs(t, m.Start("w1"), errClosed)

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	m.waitRunnersForTest()
	assert.Empty(t, f.calls, "Start after Close started a sync")
}

func TestOriginSync_SyncTick_WorkspaceLookupError_Swallowed(t *testing.T) {
	w := newFakeOriginWorkspaces() // no workspace seeded -> Get errors
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NotPanics(t, func() { require.NoError(t, m.Start("missing")) })

	// The cycle ran to completion (no panic, no wedge) and issued no fetch.
	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef called despite a workspace lookup error")
}

// TestOriginSync_TickSource_DefaultsToIntervalTicker pins the production
// wiring the seam replaces: with no test source, the cadence is a real ticker.
func TestOriginSync_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewOriginSync(context.Background(), testOriginSyncInterval, newFakeOriginWorkspaces(), newFakeOriginFetcher())

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
}

// blockingFetcher blocks each FetchRef until its context ends, then reports why.
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

// The injected 10 ms deadline is this test's SUBJECT (it stands in for
// originSyncTimeout), never its synchronisation: every wait blocks on a signal.
func TestOriginSync_SyncTick_CancelsHungFetch(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, b)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.syncTickWithTimeout(context.Background(), "w1", 10*time.Millisecond, false)
	}()

	assert.ErrorIs(t, <-b.released, context.DeadlineExceeded,
		"hung fetch must be cancelled by the per-sync timeout")
	<-done // syncTick returned once the fetch was cancelled
}

// Stop and Close cancel a hung fetch through its context instead of
// waiting out originSyncTimeout, so Close's join is prompt.
func TestOriginSync_CancelReachesAHungFetch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newFakeOriginWorkspaces()
		w.set(protectedWorkspace("w1"))
		w.set(protectedWorkspace("w2"))
		b := newBlockingFetcher()
		m := NewOriginSync(context.Background(), testOriginSyncInterval, w, b)
		t.Cleanup(m.Close)
		require.NoError(t, m.Start("w1"))
		require.NoError(t, m.Start("w2"))
		synctest.Wait()

		m.Stop("w1")
		require.ErrorIs(t, <-b.released, context.Canceled, "Stop must cancel its fetch")
		m.Close()
		require.ErrorIs(t, <-b.released, context.Canceled, "Close must cancel the fetch it joins")
	})
}

// Opening a locked root fast-forwards it onto origin's ref: the ONLY moment its
// working tree moves unasked; every tick's FetchRef touches only the ref.
func TestOriginSync_OpenFastForwardsLockedRoot(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	require.NoError(t, m.Start("w1"))
	t.Cleanup(func() { m.Stop("w1"); m.waitRunnersForTest() })

	assert.Equal(t, "/repo/w1:develop", <-g.calls, "the open sync refreshes origin/<branch> first")
	assert.Equal(t, "/repo/w1:origin/develop", <-g.ffCalls,
		"then fast-forwards the worktree onto ORIGIN's ref, not the local one")
	<-d.cycles
}

// The sync ticks only while the user is looking, so advancing on a tick would
// move files under a live editor or agent. A tick refreshes the ref, no more.
func TestOriginSync_IntervalTickDoesNotFastForward(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	require.NoError(t, m.Start("w1"))
	t.Cleanup(func() { m.Stop("w1"); m.waitRunnersForTest() })

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

// Only provisioned, locked, non-default roots advance — not the repo home, a
// placeholder, or an unlocked root that may carry local commits. All still fetch.
func TestOriginSync_OpenSkipsWorkspacesItDoesNotOwn(t *testing.T) {
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
			m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
			d := driveOriginSyncs(m)

			require.NoError(t, m.Start(ws.ID))
			t.Cleanup(func() { m.Stop(ws.ID); m.waitRunnersForTest() })

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

// git refuses rather than overwrite local changes; that must never break an open.
func TestOriginSync_OpenSurvivesRefusedFastForward(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	g.ffErr = enginegit.ErrDirtyTree
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	require.NoError(t, m.Start("w1"))
	t.Cleanup(func() { m.Stop("w1"); m.waitRunnersForTest() })

	<-g.calls
	<-g.ffCalls
	<-d.cycles // the cycle completed despite the refusal
}
