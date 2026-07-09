package projections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newRegistered builds a real asynx over a temp event store, a durable Store over
// a temp read-model DB, and registers the save-only store projection on it — the
// production shape (one RegisterStore per singleton), driven over throwaway DBs.
func newRegistered(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace], *Store) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := NewStore(db)
	require.NoError(t, err)

	require.NoError(t, RegisterStore(st, ax))
	return context.Background(), ax, st
}

func TestRegisterStore_CreatePersistsRowInReadModel(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
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

func TestRegisterStore_DeletedTombstonePersistsThenForgetDropsRow(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
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

	// Forget runs the projection's synchronous OnForget row-delete → row is gone.
	require.NoError(t, ax.Forget(ctx, "w1"))
	got, err = st.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got)
	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

// fakeAx is a minimal asynx stub that fails Subscribe / OnForget on demand.
type fakeAx struct {
	asynx.Asynx[domain.Workspace]
	subscribeErr error
	forgetErr    error
	subscribeCnt int
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Workspace],
	_ ...asynxModels.SubscriptionOpt[domain.Workspace],
) (string, error) {
	f.subscribeCnt++
	if f.subscribeCnt == 1 {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.Workspace],
) (string, error) {
	return "", f.forgetErr
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

func TestRegisterStore_SubscribeError(t *testing.T) {
	err := RegisterStore(newTestStore(t), &fakeAx{subscribeErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace store projection: subscribe")
}

func TestRegisterStore_OnForgetError(t *testing.T) {
	err := RegisterStore(newTestStore(t), &fakeAx{forgetErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace store projection: on forget")
}

// errInner fails every storage call; the projection must log and not panic.
type errInner struct{ err error }

func (e *errInner) Save(context.Context, workspaceRow) error { return e.err }
func (e *errInner) Delete(context.Context, string) error     { return e.err }
func (e *errInner) FindByKey(context.Context, string) (*workspaceRow, error) {
	return nil, e.err
}
func (e *errInner) FindAll(context.Context) ([]workspaceRow, error) { return nil, e.err }

func TestStoreProjector_SaveError_DoesNotPanic(t *testing.T) {
	p := &storeProjector{store: &Store{inner: &errInner{err: errors.New("db down")}}}
	p.onEvent(context.Background(), asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}})
}

func TestStoreProjector_ForgetError_DoesNotPanic(t *testing.T) {
	p := &storeProjector{store: &Store{inner: &errInner{err: errors.New("db down")}}}
	p.onForget(context.Background(), asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}})
}

// flakyInner fails the first failsLeft saves with a transient disk I/O error,
// then succeeds — exercising saveWithRetry's transient-retry path.
type flakyInner struct {
	failsLeft int
	saved     []workspaceRow
}

func (f *flakyInner) Save(_ context.Context, row workspaceRow) error {
	if f.failsLeft > 0 {
		f.failsLeft--
		return errors.New("disk I/O error")
	}
	f.saved = append(f.saved, row)
	return nil
}
func (f *flakyInner) Delete(context.Context, string) error { return nil }
func (f *flakyInner) FindByKey(context.Context, string) (*workspaceRow, error) {
	return nil, nil
}
func (f *flakyInner) FindAll(context.Context) ([]workspaceRow, error) { return nil, nil }

func TestStoreProjector_SaveRetriesTransientIOError(t *testing.T) {
	fi := &flakyInner{failsLeft: 2}
	p := &storeProjector{store: &Store{inner: fi}}
	p.onEvent(context.Background(), asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: "w1"}})
	require.Len(t, fi.saved, 1, "save must succeed after retrying the transient disk I/O errors")
}
