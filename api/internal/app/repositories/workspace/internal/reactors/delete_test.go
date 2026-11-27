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
	st *projections.Store,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	if st != nil {
		projections.Subscriber(st)(b)
	}
	ax, err := b.Build()
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

func noGiveUp(context.Context, string, *domain.Workspace, error) {}

type giveUp struct {
	wsID        string
	tomb        *domain.Workspace
	err, ctxErr error
}

func recordGiveUps() (GiveUp, chan giveUp) {
	calls := make(chan giveUp, 4)
	return func(ctx context.Context, wsID string, tomb *domain.Workspace, err error) {
		calls <- giveUp{wsID: wsID, tomb: tomb, err: err, ctxErr: ctx.Err()}
	}, calls
}

func TestRegisterDeleteReactor_GatedPurge_RemovesWorktreeAndForgets(t *testing.T) {
	st := newStore(t)
	ctx, ax := newAx(t, st)

	var forgot []string
	forgetDependents := func(_ context.Context, wsID string) error {
		forgot = append(forgot, wsID)
		return nil
	}
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
	require.NoError(t, RegisterDeleteReactor(ctx, ax, st, purger, gate, noGiveUp))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	rm := <-rmCh
	assert.Equal(t, "/wt/w1/worktree", rm.path, "the purge removes the tombstone's own WorktreePath")
	require.NoError(t, rm.err)
	require.NotNil(t, rm.tombstone)
	require.Equal(t, domain.WorkspaceStatusDeleted, rm.tombstone.Status)
	gate.WaitIdle(context.Background())

	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists, "aggregate must be Forgotten as the terminal purge step")
	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got, "Forget's marker must drop the read-model row")
	assert.Equal(t, []string{"w1"}, forgot, "the dependents cascade must run once")
}

func TestDeleteReactor_Gate_DoesNotPurgeUntilTombstoneObserved(t *testing.T) {
	ctx, ax := newAx(t, nil)
	reader := newGatedStoreReader()
	rmCh := make(chan string, 1)
	gate := drain.New()
	purger := NewPurger(ax, noDrop, noDependents, func(_ context.Context, tomb domain.Workspace) error { rmCh <- tomb.WorktreePath; return nil })
	require.NoError(t, RegisterDeleteReactor(ctx, ax, reader, purger, gate, noGiveUp, WithReactorTimeout(5*time.Second)))

	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	_, err := ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	<-reader.waiting
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

func TestPurger_Purge_Idempotent(t *testing.T) {
	ctx, ax := newAx(t, nil)
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

func TestPurger_Purge_PlaceholderHasNoWorktreeToRemove(t *testing.T) {
	ctx, ax := newAx(t, nil)
	createWorkspace(t, ctx, ax, "w1", "")
	rmCalled := false
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil })

	require.NoError(t, purger.Purge(ctx, domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusDeleted}))
	assert.False(t, rmCalled)
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDeleteReactor_TransientRemoveWorktreeError_RetriesUntilSuccess(t *testing.T) {
	st := newStore(t)
	ctx, ax := newAx(t, st)

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
	require.NoError(t, RegisterDeleteReactor(ctx, ax, st, purger, gate, noGiveUp, WithRetryBackoff(time.Millisecond, 4*time.Millisecond)))

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

func TestDeleteReactor_PersistentFailure_BacksOff(t *testing.T) {
	_, ax := newAx(t, nil)
	var attempts atomic.Int32
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error {
		attempts.Add(1)
		return errors.New("wedged")
	})
	r := newDeleteReactor(context.Background(), fixedStoreReader{domain.Workspace{ID: "w1", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}}, purger, drain.New(), noGiveUp,
		WithRetryBackoff(2*time.Millisecond, 16*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(ctx, "w1")

	assert.GreaterOrEqual(t, attempts.Load(), int32(4))
	assert.LessOrEqual(t, attempts.Load(), int32(20))
}

func TestDeleteReactor_TombstoneNeverObserved_DoesNotPurge(t *testing.T) {
	ctx, ax := newAx(t, nil)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	rmCalled := false
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil })
	r := newDeleteReactor(context.Background(), newGatedStoreReader(), purger, drain.New(), noGiveUp)

	bounded, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(bounded, "w1")

	assert.False(t, rmCalled, "no purge before the tombstone is observed")
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "aggregate must survive for the boot sweep to re-drive")
}

func TestDeleteReactor_AGiveUpIsReportedNotLogged(t *testing.T) {
	_, ax := newAx(t, nil)
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error {
		return errors.New("wedged")
	})
	onGiveUp, calls := recordGiveUps()
	tomb := domain.Workspace{ID: "w1", Branch: "feature/a", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}
	r := newDeleteReactor(context.Background(), fixedStoreReader{tomb}, purger, drain.New(), onGiveUp,
		WithRetryBackoff(time.Millisecond, 2*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(ctx, "w1")

	require.Len(t, calls, 1)
	got := <-calls
	require.NotNil(t, got.tomb)
	assert.Equal(t, []string{"w1", "feature/a"}, []string{got.wsID, got.tomb.Branch})
	require.ErrorContains(t, got.err, "wedged")
	assert.NoError(t, got.ctxErr, "the report must be able to save after the purge's deadline")
}

func TestDeleteReactor_ATombstoneNeverObservedReportsWithoutATomb(t *testing.T) {
	ctx, ax := newAx(t, nil)
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { return nil })
	onGiveUp, calls := recordGiveUps()
	r := newDeleteReactor(context.Background(), newGatedStoreReader(), purger, drain.New(), onGiveUp)

	bounded, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	r.purgeUntilDone(bounded, "w1")

	require.Len(t, calls, 1)
	got := <-calls
	assert.Equal(t, "w1", got.wsID)
	assert.Nil(t, got.tomb)
	require.ErrorIs(t, got.err, context.DeadlineExceeded)
}

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

func TestDeleteReactor_OnEvent_FallsBackToAggregateIDWhenEmpty(t *testing.T) {
	ctx, ax := newAx(t, nil)
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
	r := newDeleteReactor(context.Background(), reader, NewPurger(ax, noDrop, noDependents, func(_ context.Context, tomb domain.Workspace) error { rmCh <- tomb.WorktreePath; return nil }), gate, noGiveUp)

	r.onEvent(ctx, asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}})

	assert.Equal(t, "/wt/w1/worktree", <-rmCh)
	gate.WaitIdle(context.Background())
	assert.Equal(t, []string{"w1"}, awaited, "an empty AggregateID must fall back to evt.Aggregate.ID")
}

type storeReaderFunc func(ctx context.Context, id string) (domain.Workspace, error)

func (f storeReaderFunc) AwaitTombstone(ctx context.Context, id string) (domain.Workspace, error) {
	return f(ctx, id)
}

func TestDeleteReactor_OnEvent_RefusedOnceDraining(t *testing.T) {
	ctx, ax := newAx(t, nil)
	gate := drain.New()
	gate.Wait(context.Background())

	rmCalled := false
	r := newDeleteReactor(context.Background(), fixedStoreReader{}, NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { rmCalled = true; return nil }), gate, noGiveUp)
	r.onEvent(ctx, asynxModels.Event[domain.Workspace]{AggregateID: "w1"})

	assert.False(t, rmCalled, "a refused event must not spawn purge work")
}

func TestPurger_DependentsError_AbortsPurge(t *testing.T) {
	ctx, ax := newAx(t, nil)
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

func TestPurger_RemoveWorktreeError_AbortsForget(t *testing.T) {
	ctx, ax := newAx(t, nil)
	createWorkspace(t, ctx, ax, "w1", "/wt/w1/worktree")
	purger := NewPurger(ax, noDrop, noDependents, func(context.Context, domain.Workspace) error { return errors.New("permission denied") })

	require.Error(t, purger.Purge(ctx, domain.Workspace{ID: "w1", WorktreePath: "/wt/w1/worktree", Provisioning: domain.WorkspaceProvisioned}))
	exists, err := ax.Exists(ctx, "w1")
	require.NoError(t, err)
	assert.True(t, exists, "the aggregate must survive a failed rm for a re-drive")
}

type fakeAxForget struct{ forgetErr error }

func (f fakeAxForget) Forget(context.Context, string) error { return f.forgetErr }

func TestPurger_ForgetError_IsReported(t *testing.T) {
	purger := NewPurger(fakeAxForget{forgetErr: errors.New("event store unavailable")}, noDrop,
		noDependents, func(context.Context, domain.Workspace) error { return nil })

	err := purger.Purge(context.Background(), domain.Workspace{ID: "w1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event store unavailable")
}

func TestRegisterDeleteReactor_SubscribeError(t *testing.T) {
	err := RegisterDeleteReactor(
		context.Background(),
		&fakeAx{subscribeErr: errors.New("bus down")},
		fixedStoreReader{}, NewPurger(fakeAxForget{}, noDrop, noDependents, func(context.Context, domain.Workspace) error { return nil }),
		drain.New(), noGiveUp,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace delete reactor: subscribe")
}

func TestPurger_AlreadyForgottenAggregate_DropsTheOrphanedRow(t *testing.T) {
	var dropped []string
	purger := NewPurger(fakeAxForget{forgetErr: asynxModels.ErrValidation},
		func(_ context.Context, id string) error { dropped = append(dropped, id); return nil },
		noDependents, func(context.Context, domain.Workspace) error { return nil })

	require.NoError(t, purger.Purge(context.Background(), domain.Workspace{ID: "w1"}))
	assert.Equal(t, []string{"w1"}, dropped)
}
