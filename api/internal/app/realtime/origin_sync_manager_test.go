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
	// ffCalls records `merge --ff-only` invocations as "<repoPath>:<ref>", so a
	// test can assert both that an advance happened and that it targeted origin's
	// ref. Buffered like calls so a fetcher never blocks a run goroutine.
	ffCalls chan string
	// ffErr, when set, is what MergeFFOnly returns — the seam for "git refused
	// because the tree is dirty".
	ffErr error
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
	return nil
}

// testOriginSyncInterval is deliberately far longer than any test can run: no
// test waits for the cadence, and no cadence may fire behind a test's back.
// Cycles are driven explicitly through the injected tick source (originDriver),
// so the manager syncs exactly when the test says so — never on a clock. This
// matters more here than anywhere: a real origin sync holds the per-clone git
// mutex for up to 30 s, so "sleep long enough for a cycle" would be both a lie
// and unbearably slow.
const testOriginSyncInterval = time.Hour

// originDriver owns the manager's two deterministic seams.
//
//   - ticks is UNBUFFERED: sending on it is a rendezvous with the run goroutine,
//     so the send itself proves a runner is alive and has taken the tick.
//   - cycles receives once per COMPLETED cycle (the immediate-on-Acquire sync
//     included), which is the signal a negative assertion needs: "the cycle ran to
//     completion, and it fetched nothing".
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
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "develop"}
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
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "feature", ParentID: "parent-1"}
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

	// Block on the cycle actually COMPLETING, then assert it fetched nothing. The
	// old form slept 20 ms and hoped the cycle had both run and skipped by then —
	// which on a loaded machine proves neither.
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
	m.waitRunnersForTest()

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
// wiring the seam replaces: with no test source installed, run's cadence channel
// is a real interval ticker. It asserts the SOURCE, never that it fires — so
// there is nothing to wait for.
func TestOriginSyncManager_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewOriginSyncManager(
		context.Background(),
		testOriginSyncInterval,
		newFakeOriginWorkspaces(),
		newFakeOriginFetcher(),
	)

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
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

// TestOriginSyncManager_SyncTick_CancelsHungFetch is one of the two tests whose
// SUBJECT is a timeout, so a clock is intrinsic: the injected 10 ms deadline
// stands in for the production 30 s originSyncTimeout. It is not used as
// synchronisation — every wait below blocks on a real signal (the fetcher's
// released channel, the tick's own return).
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

// TestOriginSyncManager_SyncTick_DecoupledFromConnCtx also has a timeout as its
// subject (see above): the injected 10 ms deadline must be the ONLY thing that
// stops the fetch, even though the per-connection ctx is already cancelled.
func TestOriginSyncManager_SyncTick_DecoupledFromConnCtx(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, b)

	// The per-connection ctx is ALREADY cancelled, yet WithoutCancel must let the
	// fetch start (it only stops via its own timeout). This preserves the
	// best-effort background-sync guarantee while still bounding the fetch.
	connCtx, cancelConn := context.WithCancel(context.Background())
	cancelConn()

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.syncTickWithTimeout(connCtx, "w1", 10*time.Millisecond, false)
	}()

	assert.ErrorIs(t, <-b.released, context.DeadlineExceeded,
		"fetch must run under WithoutCancel and stop only on its own timeout")
	<-done
}

// ensure the fakes satisfy their interfaces at compile time.
var (
	_ OriginSyncWorkspaces = (*fakeOriginWorkspaces)(nil)
	_ OriginFetcher        = (*fakeOriginFetcher)(nil)
	_ OriginFetcher        = (*blockingFetcher)(nil)
)

// TestOriginSyncManager_OpenFastForwardsLockedRoot pins the open-time advance: a
// locked root's worktree is fast-forwarded onto origin's ref when the user opens
// it, so the files they are about to read are the files origin has.
//
// A locked root is never advanced automatically otherwise — see the tick test
// below — so this is the ONLY moment its working tree moves without the user
// asking. Nothing else in the daemon advances it: FetchRef (every tick) touches
// only refs/remotes/origin/<branch>, and a child forking off the parent
// deliberately leaves the parent's ref alone.
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

// TestOriginSyncManager_IntervalTickDoesNotFastForward is the other half of the
// contract, and the reason the advance is gated on open at all.
//
// The sync ticks only while a client is SUBSCRIBED to this workspace — i.e. only
// while the user is looking at it. Advancing on a tick would therefore move files
// under a live editor or a mid-task agent, and would do nothing during the whole
// window when advancing is harmless. The tick must refresh the ref and stop there.
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

// TestOriginSyncManager_OpenSkipsWorkspacesItDoesNotOwn proves the advance is
// confined to provisioned, locked, non-default roots. The repo home is the
// user's own folder, which Crowbar does not own; a placeholder has no worktree
// to advance; an unlocked root could carry local commits a fast-forward has no
// business touching. Each still gets its ref refreshed.
func TestOriginSyncManager_OpenSkipsWorkspacesItDoesNotOwn(t *testing.T) {
	home := lockedRootWorkspace("home")
	home.IsDefault = true
	placeholder := lockedRootWorkspace("ph")
	placeholder.WorktreePath = ""
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

// TestOriginSyncManager_OpenSurvivesRefusedFastForward proves a refused advance
// is inert. git refuses rather than overwrite local changes, which is exactly
// why no pre-flight cleanliness check is needed — and a refusal must never break
// opening a workspace.
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
