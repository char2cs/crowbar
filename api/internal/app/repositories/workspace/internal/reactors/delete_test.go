package reactors

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
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
		WithSnapshotStore(asynxstore.NewSnapshots()).
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

// gatedStoreReader holds AwaitTombstone until the test releases a tombstone, so
// a test controls exactly when the persisted deleted row becomes visible.
type gatedStoreReader struct {
	waiting chan struct{}
	release chan domain.Workspace
}

func newGatedStoreReader() *gatedStoreReader {
	return &gatedStoreReader{waiting: make(chan struct{}, 1), release: make(chan domain.Workspace, 1)}
}

func (g *gatedStoreReader) AwaitTombstone(
	ctx context.Context,
	_ string,
) (domain.Workspace, error) {
	select {
	case g.waiting <- struct{}{}:
	default:
	}
	select {
	case tomb := <-g.release:
		return tomb, nil
	case <-ctx.Done():
		return domain.Workspace{}, ctx.Err()
	}
}

// fixedStoreReader answers every AwaitTombstone with the same tombstone.
type fixedStoreReader struct{ tomb domain.Workspace }

func (f fixedStoreReader) AwaitTombstone(context.Context, string) (domain.Workspace, error) {
	return f.tomb, nil
}

func createWorkspace(
	t *testing.T,
	ctx context.Context,
	ax asynx.Asynx[domain.Workspace],
	id string,
	worktreePath string,
) {
	t.Helper()
	provisioning := domain.WorkspaceProvisioned
	if worktreePath == "" {
		provisioning = domain.WorkspacePlaceholder
	}
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID:           id,
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "main",
		WorktreePath: worktreePath,
		Now:          time.Unix(1, 0).UTC(),
		Provisioning: provisioning,
	})
	require.NoError(t, err)
}

func noDependents(context.Context, string) error { return nil }

func noDrop(context.Context, string) error { return nil }

// TestRegisterDeleteReactor_GatedPurge_RemovesWorktreeAndForgets drives a real
// Delete through a real axWorkspace with the real save-only store projection:
// the projection persists the "deleted" tombstone (waking the reactor), the
// reactor cascades the dependents, removes the tombstone's own worktree path and
// Forgets the aggregate, which drops the read-model row (spec §3.6/§3.8).
func TestRegisterDeleteReactor_GatedPurge_RemovesWorktreeAndForgets(t *testing.T) {
	ctx, ax := newAx(t)
	st := newStore(t)
	require.NoError(t, projections.RegisterStore(st, ax))

	var forgot []string
	forgetDependents := func(_ context.Context, wsID string) error {
		forgot = append(forgot, wsID)
		return nil
	}
	// The reactor runs concurrently with this test, so the tombstone row is
	// observed where the invariant lives: at the moment the worktree is removed
	// (persist happens-before purge), not by racing a read against the purge.
	type removal struct {
		path      string
		tombstone *domain.Workspace
		err       error
	}
	rmCh := make(chan removal, 1)
	rmWorktree := func(rmCtx context.Context, tomb domain.Workspace) error {
		row, err := st.Get(rmCtx, "w1")
		rmCh <- removal{path: tomb.WorktreePath, tombstone: row, err: err}
		return nil
	}
	gate := drain.New()
	purger := NewPurger(ax, noDrop, forgetDependents, rmWorktree)
	require.NoError(t, RegisterDeleteReactor(ax, st, purger, gate))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	// rmWorktree pushes onto rmCh: a genuine completion signal, so block on it
	// directly (a hang would surface via go test -timeout).
	rm := <-rmCh
	assert.Equal(t, "/wt/w1/worktree", rm.path, "the purge removes the tombstone's own WorktreePath")
	// The deleted tombstone row was persisted before the purge began (the boot
	// orphan-sweep depends on this row surviving a crash mid-purge).
	require.NoError(t, rm.err)
	require.NotNil(t, rm.tombstone)
	require.Equal(t, domain.WorkspaceStatusDeleted, rm.tombstone.Status)
	gate.WaitIdle(context.Background())

	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists, "aggregate must be Forgotten as the terminal purge step")
	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got, "Forget's synchronous OnForget must drop the read-model row")
	assert.Equal(t, []string{"w1"}, forgot, "the dependents cascade must run once")
}

// TestDeleteReactor_Gate_DoesNotPurgeUntilTombstoneObserved proves the ordering
// gate: until the persisted tombstone is observed, nothing is purged (spec §3.6
// persist-happens-before-purge).
func TestDeleteReactor_Gate_DoesNotPurgeUntilTombstoneObserved(t *testing.T) {
	ctx, ax := newAx(t)
	reader := newGatedStoreReader()
	rmCh := make(chan string, 1)
	gate := drain.New()
	purger := NewPurger(ax, noDrop, noDependents, func(_ context.Context, tomb domain.Workspace) error { rmCh <- tomb.WorktreePath; return nil })
	require.NoError(t, RegisterDeleteReactor(ax, reader, purger, gate, WithReactorTimeout(5*time.Second)))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	<-reader.waiting // the reactor is live and parked on the gate
	select {
	case <-rmCh:
		t.Fatal("worktree removed before the deleted row was observed")
	default:
	}

	reader.release <- domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted, WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}
	assert.Equal(t, "/wt/w1/worktree", <-rmCh)
	gate.WaitIdle(context.Background())
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestPurger_Purge_Idempotent re-drives the purge over an already-purged
// aggregate: the boot sweep re-runs the same Purge verbatim after a crash, so
// every step must converge (spec §3.6 idempotency contract).
func TestPurger_Purge_Idempotent(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	tomb := domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted, WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}
	rmCount := 0
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCount++; return nil })

	require.NoError(t, purger.Purge(ctx, tomb))
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, purger.Purge(ctx, tomb), "a re-drive over a Forgotten aggregate is a no-op")
	assert.Equal(t, 2, rmCount, "the remover itself is idempotent over an absent root")
}

// An unprovisioned placeholder has no worktree of its own: nothing to remove,
// but its dependents and its aggregate still go.
func TestPurger_Purge_PlaceholderHasNoWorktreeToRemove(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "")
	rmCalled := false
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil })

	require.NoError(t, purger.Purge(ctx, domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted}))
	assert.False(t, rmCalled)
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestDeleteReactor_TransientRemoveWorktreeError_RetriesUntilSuccess proves a
// transient rm failure does not abandon the purge until the next restart: the
// reactor retries the idempotent sequence until it succeeds.
func TestDeleteReactor_TransientRemoveWorktreeError_RetriesUntilSuccess(t *testing.T) {
	ctx, ax := newAx(t)
	st := newStore(t)
	require.NoError(t, projections.RegisterStore(st, ax))

	var attempts atomic.Int32
	rmCh := make(chan string, 1)
	purger := NewPurger(ax, noDrop, noDependents, func(_ context.Context, tomb domain.Workspace) error {
		if attempts.Add(1) <= 2 {
			return errors.New("transiently busy")
		}
		rmCh <- tomb.WorktreePath
		return nil
	})
	gate := drain.New()
	require.NoError(t, RegisterDeleteReactor(ax, st, purger, gate, WithRetryBackoff(time.Millisecond, 4*time.Millisecond)))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	assert.Equal(t, "/wt/w1/worktree", <-rmCh)
	gate.WaitIdle(context.Background())
	assert.Equal(t, int32(3), attempts.Load(), "retried past the transient failures")
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
}

// A persistent failure must back off, not spin: the old fixed 25 ms retry made
// ~4,800 rm attempts (each with an ERROR log) over the reactor's lifetime. With
// exponential backoff the attempts over a window grow only logarithmically
// until the cap, then linearly at the cap.
func TestDeleteReactor_PersistentFailure_BacksOff(t *testing.T) {
	_, ax := newAx(t)
	var attempts atomic.Int32
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error {
		attempts.Add(1)
		return errors.New("wedged")
	})
	r := newDeleteReactor(fixedStoreReader{domain.Workspace{ID: "w1", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}}, purger, drain.New(),
		WithRetryBackoff(2*time.Millisecond, 16*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(ctx, "w1")

	// 2+4+8+16 = 30ms for the first 5 attempts, then one per 16ms: ~12 in 150ms.
	// A fixed 2ms retry would have made ~75.
	assert.GreaterOrEqual(t, attempts.Load(), int32(4))
	assert.LessOrEqual(t, attempts.Load(), int32(20))
}

// TestDeleteReactor_TombstoneNeverObserved_DoesNotPurge asserts that if the
// deleted row is never observed within the deadline, the reactor defers to the
// boot sweep: no rm, no Forget, so no orphan-without-row is ever created.
func TestDeleteReactor_TombstoneNeverObserved_DoesNotPurge(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	rmCalled := false
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil })
	r := newDeleteReactor(newGatedStoreReader(), purger, drain.New())

	bounded, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(bounded, "w1")

	assert.False(t, rmCalled, "no purge before the tombstone is observed")
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "aggregate must survive for the boot sweep to re-drive")
}

// fakeAx is a minimal asynx stub whose Subscribe fails on demand.
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

// TestDeleteReactor_OnEvent_FallsBackToAggregateIDWhenEmpty proves onEvent's id
// resolution: some event sources leave AggregateID unset.
func TestDeleteReactor_OnEvent_FallsBackToAggregateIDWhenEmpty(t *testing.T) {
	ctx, ax := newAx(t)
	var mu sync.Mutex
	var awaited []string
	reader := storeReaderFunc(func(_ context.Context, id string) (domain.Workspace, error) {
		mu.Lock()
		awaited = append(awaited, id)
		mu.Unlock()
		return domain.Workspace{ID: id, WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}, nil
	})
	rmCh := make(chan string, 1)
	gate := drain.New()
	r := newDeleteReactor(reader, NewPurger(ax, noDrop, noDependents, func(_ context.Context, tomb domain.Workspace) error { rmCh <- tomb.WorktreePath; return nil }), gate)

	r.onEvent(ctx, asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}})

	assert.Equal(t, "/wt/w1/worktree", <-rmCh)
	gate.WaitIdle(context.Background())
	assert.Equal(t, []string{"w1"}, awaited, "an empty AggregateID must fall back to evt.Aggregate.ID")
}

type storeReaderFunc func(ctx context.Context, id string) (domain.Workspace, error)

func (f storeReaderFunc) AwaitTombstone(ctx context.Context, id string) (domain.Workspace, error) {
	return f(ctx, id)
}

// TestDeleteReactor_OnEvent_RefusedOnceDraining proves the gate stops the
// reactor from spawning new purge work once a drain has begun: a refused event
// is not a dropped one (the boot sweep re-purges), but it must not touch disk.
func TestDeleteReactor_OnEvent_RefusedOnceDraining(t *testing.T) {
	ctx, ax := newAx(t)
	gate := drain.New()
	gate.Wait(context.Background())

	rmCalled := false
	r := newDeleteReactor(fixedStoreReader{}, NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil }), gate)
	r.onEvent(ctx, asynxModels.Event[domain.Workspace]{AggregateID: "w1"})

	assert.False(t, rmCalled, "a refused event must not spawn purge work")
}

// TestPurger_DependentsError_AbortsPurge proves the ordering contract: if the
// dependents cascade fails, the worktree is not removed and the aggregate is not
// forgotten — a re-drive must still be able to run the cascade.
func TestPurger_DependentsError_AbortsPurge(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	rmCalled := false
	purger := NewPurger(ax, noDrop,
		func(context.Context, string) error { return errors.New("cascade failed") },
		func(context.Context, domain.Workspace) error { rmCalled = true; return nil })

	require.Error(t, purger.Purge(ctx, domain.Workspace{ID: "w1", WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}))
	assert.False(t, rmCalled)
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "the aggregate must survive a cascade failure for a re-drive")
}

// TestPurger_RemoveWorktreeError_AbortsForget proves a failed rm leaves the
// tombstone in place, so a re-drive retries the removal.
func TestPurger_RemoveWorktreeError_AbortsForget(t *testing.T) {
	ctx, ax := newAx(t)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { return errors.New("permission denied") })

	require.Error(t, purger.Purge(ctx, domain.Workspace{ID: "w1", WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}))
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "the aggregate must survive a failed rm for a re-drive")
}

// fakeAxForget overrides Forget so a test can force a real (non-ErrValidation)
// failure of the terminal step.
type fakeAxForget struct{ forgetErr error }

func (f fakeAxForget) Forget(context.Context, string) error { return f.forgetErr }

// A real Forget failure is reported, never swallowed: it is the last chance to
// notice the aggregate was never actually forgotten.
func TestPurger_ForgetError_IsReported(t *testing.T) {
	purger := NewPurger(fakeAxForget{forgetErr: errors.New("event store unavailable")}, noDrop,
		noDependents, func(context.Context, domain.Workspace) error { return nil })

	err := purger.Purge(context.Background(), domain.Workspace{ID: "w1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event store unavailable")
}

func TestRegisterDeleteReactor_SubscribeError(t *testing.T) {
	err := RegisterDeleteReactor(
		&fakeAx{subscribeErr: errors.New("bus down")},
		fixedStoreReader{}, NewPurger(fakeAxForget{}, noDrop, noDependents, func(context.Context, domain.Workspace) error { return nil }),
		drain.New(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace delete reactor: subscribe")
}

// A crash between Forget and the row delete its OnForget publishes leaves a
// tombstone row whose aggregate is already gone. Forget now answers
// ErrValidation and no projection will ever delete that row, so the purge drops
// it itself — otherwise every boot sweep re-finds it forever.
func TestPurger_AlreadyForgottenAggregate_DropsTheOrphanedRow(t *testing.T) {
	var dropped []string
	purger := NewPurger(fakeAxForget{forgetErr: asynxModels.ErrValidation},
		func(_ context.Context, id string) error { dropped = append(dropped, id); return nil },
		noDependents, func(context.Context, domain.Workspace) error { return nil })

	require.NoError(t, purger.Purge(context.Background(), domain.Workspace{ID: "w1"}))
	assert.Equal(t, []string{"w1"}, dropped)
}
