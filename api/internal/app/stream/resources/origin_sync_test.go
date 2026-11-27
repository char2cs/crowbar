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

type fakeOriginFetcher struct {
	calls   chan string
	ffCalls chan string
	ffErr   error
	hold    chan struct{}
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

const testOriginSyncInterval = time.Hour

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

func (d *originDriver) tick() {
	d.ticks <- time.Time{}
	<-d.cycles
}

func protectedWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "develop", Provisioning: domain.WorkspaceProvisioned}
}

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

	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef fired for a workspace with a parent")
}

func TestOriginSync_Start_SyncsImmediately(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
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
	<-f.calls
	<-d.cycles

	m.Stop("w1")
	m.waitRunnersForTest()

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
	require.NotPanics(t, m.Close)

	m.mu.Lock()
	count := len(m.cancels)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

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

func TestOriginSync_CancelBeforeTheFastForwardSkipsIt(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	g.hold = make(chan struct{})
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	require.NoError(t, m.Start("w1"))
	<-g.calls

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
	w := newFakeOriginWorkspaces()
	f := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, f)
	d := driveOriginSyncs(m)
	t.Cleanup(m.Close)

	require.NotPanics(t, func() { require.NoError(t, m.Start("missing")) })

	<-d.cycles
	assert.Empty(t, f.calls, "FetchRef called despite a workspace lookup error")
}

func TestOriginSync_TickSource_DefaultsToIntervalTicker(t *testing.T) {
	m := NewOriginSync(context.Background(), testOriginSyncInterval, newFakeOriginWorkspaces(), newFakeOriginFetcher())

	tickC, stop := m.tickSource()
	require.NotNil(t, tickC)
	stop()
}

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
	<-done
}

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

func TestOriginSync_IntervalTickDoesNotFastForward(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(lockedRootWorkspace("w1"))
	g := newFakeOriginFetcher()
	m := NewOriginSync(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)

	require.NoError(t, m.Start("w1"))
	t.Cleanup(func() { m.Stop("w1"); m.waitRunnersForTest() })

	<-g.calls
	<-g.ffCalls
	<-d.cycles

	d.ticks <- time.Now()
	assert.Equal(t, "/repo/w1:develop", <-g.calls, "the tick still refreshes the origin ref")
	<-d.cycles
	assert.Empty(t, g.ffCalls, "...and it fast-forwarded nothing")
}

func TestOriginSync_OpenSkipsWorkspacesItDoesNotOwn(t *testing.T) {
	home := lockedRootWorkspace("home")
	home.IsDefault = true
	home.Provisioning = domain.WorkspaceShared
	placeholder := lockedRootWorkspace("ph")
	placeholder.WorktreePath = ""
	placeholder.Provisioning = domain.WorkspacePlaceholder
	unlocked := protectedWorkspace("unlocked")

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
				<-g.calls
			}
			<-d.cycles
			assert.Empty(t, g.ffCalls, "this workspace must never be fast-forwarded")
		})
	}
}

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
	<-d.cycles
}
