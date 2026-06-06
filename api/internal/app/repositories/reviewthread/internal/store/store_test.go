package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	rtcmds "github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.ReviewThread]) {
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

	st, err := store.New(db, ax, func(domain.ReviewThread) {})
	require.NoError(t, err)
	return context.Background(), st, ax
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
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(db, ax, func(domain.ReviewThread) {})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListByWorkspace(context.Background(), "w1")
	require.Error(t, err)
}
