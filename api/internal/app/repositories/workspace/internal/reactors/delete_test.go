package reactors

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

func newAx(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), ax
}

func newStore(
	t *testing.T,
) *projections.Store {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := projections.NewStore(db)
	require.NoError(t, err)
	return st
}

// fakeStoreReader lets a test control exactly when the persisted deleted row
// becomes visible to the reactor's ordering gate. When polled is non-nil, each
// Get non-blockingly signals it, giving a test a deterministic "the gate has
// actually re-read the read model" barrier in place of a wall-clock wait.
type fakeStoreReader struct {
	mu     sync.Mutex
	ws     *domain.Workspace
	polled chan struct{}
}

func (f *fakeStoreReader) set(
	ws *domain.Workspace,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ws = ws
}

func (f *fakeStoreReader) Get(
	_ context.Context,
	_ string,
) (*domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.polled != nil {
		select {
		case f.polled <- struct{}{}:
		default:
		}
	}
	if f.ws == nil {
		return nil, nil
	}
	cp := *f.ws
	return &cp, nil
}

// fakePaths is an in-memory wspaths.WorkspacePaths double that records Delete
// calls and returns wspaths.ErrNotFound for missing rows, mirroring the real
// store's boundary contract.
type fakePaths struct {
	mu          sync.Mutex
	rows        map[string]string
	deleteCalls []string
}

func newFakePaths() *fakePaths {
	return &fakePaths{rows: map[string]string{}}
}

func (f *fakePaths) Put(
	_ context.Context,
	workspaceID string,
	worktreePath string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[workspaceID] = worktreePath
	return nil
}

func (f *fakePaths) Get(
	_ context.Context,
	workspaceID string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path, ok := f.rows[workspaceID]
	if !ok {
		return "", wspaths.ErrNotFound
	}
	return path, nil
}

func (f *fakePaths) Delete(
	_ context.Context,
	workspaceID string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, workspaceID)
	delete(f.rows, workspaceID)
	return nil
}

func (f *fakePaths) deletes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleteCalls...)
}

func createWorkspace(
	t *testing.T,
	ctx context.Context,
	ax asynx.Asynx[domain.Workspace],
	id string,
	worktreePath string,
) {
	t.Helper()
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID:           id,
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "main",
		WorktreePath: worktreePath,
		Now:          time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
}

// TestRegisterDeleteReactor_GatedPurge_RemovesWorktreeAndForgets drives a real
// Delete through a real axWorkspace with the real save-only store projection
// registered: the projection persists the "deleted" tombstone row, the reactor
// observes it, then cascades the review-thread Forget, removes the worktree,
// deletes the id↔path row, and Forgets the aggregate (dropping the read-model
// row). This is the end-to-end persist-then-purge lifecycle (spec §3.6/§3.8).
func TestRegisterDeleteReactor_GatedPurge_RemovesWorktreeAndForgets(t *testing.T) {
	ctx, ax := newAx(t)
	st := newStore(t)
	require.NoError(t, projections.RegisterStore(st, ax))

	paths := newFakePaths()
	require.NoError(t, paths.Put(ctx, "w1", "/wt/w1"))

	var rtForgot []string
	reviewThreadForget := func(_ context.Context, wsID string) error {
		rtForgot = append(rtForgot, wsID)
		return nil
	}
	rmCh := make(chan string, 1)
	rmWorktree := func(path string) error {
		rmCh <- path
		return nil
	}
	gate := drain.New()

	require.NoError(t, RegisterDeleteReactor(ax, st, paths, reviewThreadForget, rmWorktree, gate))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1")
	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)

	_, err = ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	// The store projection persisted the deleted tombstone row before the reactor
	// purges it (the boot orphan-sweep depends on this row surviving a crash).
	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.WorkspaceStatusDeleted, got.Status)

	// rmWorktree pushes the removed path onto rmCh: it is a genuine completion
	// signal, so block on it directly (a hang would surface via go test -timeout).
	path := <-rmCh
	assert.Equal(t, "/wt/w1", path)
	gate.WaitIdle(context.Background())

	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists, "aggregate must be Forgotten as the terminal purge step")

	got, err = st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got, "Forget's synchronous OnForget must drop the read-model row")

	assert.Equal(t, []string{"w1"}, rtForgot, "review-thread cascade must run once")
	assert.Equal(t, []string{"w1"}, paths.deletes(), "the id↔path row must be deleted")
}

// TestDeleteReactor_Gate_DoesNotPurgeUntilTombstoneObserved proves the ordering
// gate: with no persisted deleted row visible, the reactor must NOT rm the
// worktree; only once the tombstone becomes observable does the purge proceed
// (spec §3.6 persist-happens-before-purge).
func TestDeleteReactor_Gate_DoesNotPurgeUntilTombstoneObserved(t *testing.T) {
	ctx, ax := newAx(t)

	reader := &fakeStoreReader{polled: make(chan struct{}, 1)}
	paths := newFakePaths()
	require.NoError(t, paths.Put(ctx, "w1", "/wt/w1"))

	rmCh := make(chan string, 1)
	rmWorktree := func(path string) error {
		rmCh <- path
		return nil
	}
	gate := drain.New()

	require.NoError(t, RegisterDeleteReactor(
		ax, reader, paths,
		func(context.Context, string) error { return nil },
		rmWorktree, gate,
		WithGatePollInterval(5*time.Millisecond),
		WithReactorTimeout(5*time.Second),
	))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	// Block until the ordering gate has actually re-read the (still empty) read
	// model at least once: this proves the reactor is live and parked on the gate.
	// It cannot have purged yet — rm follows only an observed tombstone, which is
	// not set — so the emptiness of rmCh is a deterministic non-blocking check.
	<-reader.polled
	select {
	case <-rmCh:
		t.Fatal("worktree removed before the deleted row was observed")
	default:
	}

	reader.set(&domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted, WorktreePath: "/wt/w1"})

	// The tombstone is now observable; rmWorktree's push onto rmCh is a genuine
	// completion signal, so block on it directly.
	path := <-rmCh
	assert.Equal(t, "/wt/w1", path)
	gate.WaitIdle(context.Background())

	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, []string{"w1"}, paths.deletes())
}

// TestDeleteReactor_Purge_Idempotent re-drives the purge over an already-purged
// aggregate: the boot sweep re-runs the same reactor verbatim after a crash, so
// Forgetting an already-Forgotten aggregate, deleting a gone path row, and
// rm-ing an absent worktree must all be no-ops that converge to the same end
// state (spec §3.6 idempotency contract).
func TestDeleteReactor_Purge_Idempotent(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1")

	reader := &fakeStoreReader{}
	reader.set(&domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted})
	paths := newFakePaths()
	require.NoError(t, paths.Put(ctx, "w1", "/wt/w1"))

	rmCount := 0
	rmWorktree := func(string) error {
		rmCount++
		return nil
	}
	gate := drain.New()
	r := newDeleteReactor(
		ax, reader, paths,
		func(context.Context, string) error { return nil },
		rmWorktree, gate,
		WithGatePollInterval(2*time.Millisecond),
	)

	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	r.purge(bounded, "w1")
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	require.False(t, exists)
	require.Equal(t, 1, rmCount)

	// Re-drive: aggregate already Forgotten, path row already gone.
	r.purge(bounded, "w1")
	exists, err = ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, 1, rmCount, "an absent worktree must not be rm'd again")
}

// TestDeleteReactor_Gate_TimesOutWithoutPurge asserts that if the deleted row is
// never observed within the bounded deadline (e.g. the store projection lost the
// write), the reactor defers to the boot sweep: it does NOT rm or Forget, so no
// orphan-without-row is ever created.
func TestDeleteReactor_Gate_TimesOutWithoutPurge(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1")

	reader := &fakeStoreReader{} // never returns a deleted row
	paths := newFakePaths()
	require.NoError(t, paths.Put(ctx, "w1", "/wt/w1"))

	rmCalled := false
	rmWorktree := func(string) error {
		rmCalled = true
		return nil
	}
	gate := drain.New()
	r := newDeleteReactor(
		ax, reader, paths,
		func(context.Context, string) error { return nil },
		rmWorktree, gate,
		WithGatePollInterval(2*time.Millisecond),
	)

	bounded, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	r.purge(bounded, "w1")

	assert.False(t, rmCalled, "no purge before the tombstone is observed")
	assert.Empty(t, paths.deletes())
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "aggregate must survive for the boot sweep to re-drive")
}

// fakeAx is a minimal asynx stub whose Subscribe fails on demand, exercising the
// registration error path.
type fakeAx struct {
	asynx.Asynx[domain.Workspace]
	subscribeErr error
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Workspace],
	_ ...asynxModels.SubscriptionOpt[domain.Workspace],
) (string, error) {
	return "", f.subscribeErr
}

func TestRegisterDeleteReactor_SubscribeError(t *testing.T) {
	gate := drain.New()
	err := RegisterDeleteReactor(
		&fakeAx{subscribeErr: errors.New("bus down")},
		&fakeStoreReader{}, newFakePaths(),
		func(context.Context, string) error { return nil },
		func(string) error { return nil },
		gate,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace delete reactor: subscribe")
}
