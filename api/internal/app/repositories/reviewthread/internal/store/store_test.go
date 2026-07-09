package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	rtcmds "github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStoreWithDeps(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.ReviewThread], *gormdb.DB) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(domain.ReviewThread) {})
	require.NoError(t, err)
	return context.Background(), st, ax, db
}

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.ReviewThread]) {
	t.Helper()
	ctx, st, ax, _ := newStoreWithDeps(t)
	return ctx, st, ax
}

func TestStore_ListReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

// TestStore_ListRebuildsWhenModelEmptyButLogNonEmpty proves the lazy Replay repair
// (spec §3.7, decision 7) replacing the retired eager reconcile-on-open: after the
// durable read model is lost but the event log survives, List enumerates every
// aggregate id via the event store's AggregateLister and Replays each back into
// state/store/review_thread.db.
func TestStore_ListRebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)

	for _, id := range []string{"t1", "t2"} {
		_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
			ID: id, WsID: "w1", MessageID: "m-" + id, Now: time.Unix(1, 0).UTC(),
		})
		require.NoError(t, err)
	}

	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2, "read model populated by the live store projection")

	// Simulate read-model loss with the event log intact.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_review_threads").Error)

	// The per-request List heals the model via whole-model lazy Replay.
	rebuilt, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, rebuilt, 2, "List must Replay every id from the event log")
	require.ElementsMatch(t, []string{"t1", "t2"}, []string{rebuilt[0].ID, rebuilt[1].ID})
}

// TestStore_ListEmptyLogReturnsEmpty proves that with a genuinely empty event log
// there is nothing to Replay: List returns an empty list without error.
func TestStore_ListEmptyLogReturnsEmpty(t *testing.T) {
	ctx, st, _ := newStore(t)
	rows, err := st.List(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestStore_GetReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t2", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	got, err := st.Get(ctx, "t2")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.ReviewThreadStatusOpen, got.Status)
}

func TestStore_GetMissingReturnsNil(t *testing.T) {
	ctx, st, _ := newStore(t)
	got, err := st.Get(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_ListByWorkspace_FiltersByWsID(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t2", WsID: "w2", MessageID: "m2", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "t1", list[0].ID)
}

func TestStore_ListByWorkspace_NonMatchReturnsEmpty(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_ListByWorkspace_StorageError(t *testing.T) {
	ctx, st, _, db := newStoreWithDeps(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListByWorkspace(ctx, "w1")
	require.Error(t, err)
}

// fakeReplayES is a serialize.AggregateLister whose AggregateIDs errors, so a
// List over an empty read model surfaces the rebuild's enumeration failure.
type fakeReplayES struct {
	asynxModels.Store
	err error
}

func (f *fakeReplayES) AggregateIDs(context.Context) ([]string, error) {
	return nil, f.err
}

func TestStore_List_RebuildEnumerationError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	// Inject an event log whose AggregateIDs enumeration fails; an empty read model
	// then routes List into rebuild, which surfaces the enumeration error.
	st, err := store.New(db, &fakeReplayES{err: errors.New("boom")}, ax, func(domain.ReviewThread) {})
	require.NoError(t, err)

	_, err = st.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}
