package projections

import (
	"context"
	"errors"
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
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newRegistered builds a real asynx over a temp event store with the store
// projection as its durable subscriber, the production shape.
func newRegistered(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace], *Store) {
	t.Helper()
	st := newTestStore(t)
	ctx, ax := buildOver(t, st)
	return ctx, ax, st
}

func buildOver(
	t *testing.T,
	st *Store,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithDeliveryRetry(time.Millisecond, time.Millisecond)
	Subscriber(st)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	return context.Background(), ax
}

func TestWorkspaceProjection_CreatePersistsRowInReadModel(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "w1", all[0].ID)
	assert.Equal(t, "main", all[0].Branch)
	// The read model carries project_id/repo_id off evt.Aggregate — it doubles as
	// the location index (spec §3.7).
	assert.Equal(t, "p1", all[0].ProjectID)
	assert.Equal(t, "r1", all[0].RepoID)

	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
}

func TestWorkspaceProjection_ForgetDeletesTheRow(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)

	// The Delete command folds Status=deleted; the store projection PERSISTS that
	// tombstone row (it does NOT Forget synchronously), so the boot orphan-sweep
	// still has a row to find (spec §3.8 delete lifecycle).
	_, err = ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)
	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.WorkspaceStatusDeleted, got.Status)

	// Forget delivers its marker to the durable subscriber, which drops the row.
	require.NoError(t, ax.Forget(ctx, "w1"))
	got, err = st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got)
	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

// failFirstSave fails the first Save, then signals every Save that succeeds.
type failFirstSave struct {
	inner
	failed atomic.Bool
	saved  chan struct{}
}

type inner interface {
	Save(ctx context.Context, row workspaceRow) error
	Delete(ctx context.Context, key string) error
	FindByKey(ctx context.Context, key string) (*workspaceRow, error)
	FindAll(ctx context.Context) ([]workspaceRow, error)
}

func (f *failFirstSave) Save(ctx context.Context, row workspaceRow) error {
	if f.failed.CompareAndSwap(false, true) {
		return errors.New("injected save failure")
	}
	if err := f.inner.Save(ctx, row); err != nil {
		return err
	}
	f.saved <- struct{}{}
	return nil
}

func TestWorkspaceProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) {
	st := newTestStore(t)
	flaky := &failFirstSave{inner: st.inner, saved: make(chan struct{}, 16)}
	st.inner = flaky
	ctx, ax := buildOver(t, st)

	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)

	select {
	case <-flaky.saved:
	case <-time.After(10 * time.Second):
		t.Fatal("the failed save was never retried")
	}
	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "main", got.Branch)
}

// Upstream can redeliver a Forget marker at startup; the second one finds no row.
func TestWorkspaceProjection_AForgetDeliveredTwiceIsANoOp(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.save(ctx, domain.Workspace{ID: "w1"}))
	marker := asynxModels.Event[domain.Workspace]{AggregateID: "w1", EventName: asynxModels.ForgetEventName}

	require.NoError(t, st.handle(ctx, marker))
	require.NoError(t, st.handle(ctx, marker))

	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func newTestStore(
	t *testing.T,
) *Store {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := NewStore(db)
	require.NoError(t, err)
	return st
}

// errInner fails every storage call.
type errInner struct{ err error }

func (e *errInner) Save(context.Context, workspaceRow) error { return e.err }
func (e *errInner) Delete(context.Context, string) error     { return e.err }
func (e *errInner) FindByKey(context.Context, string) (*workspaceRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(context.Context) ([]workspaceRow, error) { return nil, e.err }

func TestWorkspaceProjection_AStorageFailureIsReturned(t *testing.T) {
	st := &Store{inner: &errInner{err: errors.New("db down")}, announced: map[string]struct{}{}}
	ctx := context.Background()
	require.Error(t, st.handle(ctx, asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}}))
	assert.Error(t, st.handle(ctx, asynxModels.Event[domain.Workspace]{AggregateID: "w1", EventName: asynxModels.ForgetEventName}))
}

func TestNewStore_ErrorOnBadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = NewStore(db)

	assert.Error(t, err)
}

func TestStore_List_ErrorOnStorageFailure(t *testing.T) {
	st := &Store{inner: &errInner{err: errors.New("db down")}}

	_, err := st.List(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "find all")
}

func TestStore_Get_ErrorOnStorageFailure(t *testing.T) {
	st := &Store{inner: &errInner{err: errors.New("db down")}}

	_, err := st.Get(context.Background(), "w1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "find")
}

// corruptInner returns rows whose Data is not valid JSON, simulating a
// read-model row written by an incompatible/older version of the schema.
type corruptInner struct{}

func (corruptInner) Save(context.Context, workspaceRow) error { return nil }
func (corruptInner) Delete(context.Context, string) error     { return nil }
func (corruptInner) FindByKey(context.Context, string) (*workspaceRow, error) {
	return &workspaceRow{ID: "w1", Data: []byte("not valid json")}, nil
}

func (corruptInner) FindAll(context.Context) ([]workspaceRow, error) {
	return []workspaceRow{{ID: "w0", Data: []byte(`{"id":"w0"}`)}, {ID: "w1", Data: []byte("not valid json")}}, nil
}

func TestStore_List_SkipsACorruptRow(t *testing.T) {
	st := &Store{inner: corruptInner{}}

	got, err := st.List(t.Context())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "w0", got[0].ID)
}

func TestStore_Get_ErrorOnCorruptRow(t *testing.T) {
	st := &Store{inner: corruptInner{}}

	_, err := st.Get(context.Background(), "w1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}
