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
	chatcmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(context.Background(), db, ax, func(domain.Chat) {}, nil)
	require.NoError(t, err)
	return context.Background(), st, ax
}

func TestStore_ListReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{
		ID: "c1", WsID: "w1", Title: "hi", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

// TestStore_ReconcilesReadModelFromEventStoreOnOpen proves R5 for the global chat
// store: if a projection is dropped (a crash between the durable event Append and
// the async projection), the read model is missing the row while the event store
// has it. Opening a fresh read model over that event store with the aggregate id
// in aggregateIDs must reconcile — otherwise List stays permanently wrong while
// Get is correct.
func TestStore_ReconcilesReadModelFromEventStoreOnOpen(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ctx := context.Background()
	db1, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	_, err = store.New(ctx, db1, ax, func(domain.Chat) {}, nil)
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, chatcmds.CreateChat{
		ID: "c1", WsID: "w1", Title: "hi", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	// A FRESH, empty read model over the SAME event store — the read model a daemon
	// would see after a crash dropped c1's projection. The projection bus of this
	// new store never saw c1's create, so only reconcile-on-open can recover it.
	db2, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax2, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax2.Shutdown(context.Background()) })

	st2, err := store.New(ctx, db2, ax2, func(domain.Chat) {}, []string{"c1"})
	require.NoError(t, err)

	all, err := st2.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "reconcile-on-open must heal the read model from the event store after a dropped projection")
	assert.Equal(t, "c1", all[0].ID)
	assert.Equal(t, "hi", all[0].Title)
}

func TestStore_GetReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{
		ID: "c2", WsID: "w1", Title: "greet", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	got, err := st.Get(ctx, "c2")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "greet", got.Title)
}

func TestStore_GetMissingReturnsNil(t *testing.T) {
	ctx, st, _ := newStore(t)
	got, err := st.Get(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_ListByWorkspace_FiltersByWsID(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, chatcmds.CreateChat{ID: "c2", WsID: "w2", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)
}

func TestStore_ListByWorkspace_ExcludesSoftDeleted(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, chatcmds.CreateChat{ID: "c2", WsID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, chatcmds.DeleteChat{ID: "c2", Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)
}

func TestStore_Get_ReturnsDeletedRows(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, chatcmds.DeleteChat{ID: "c1", Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)

	got, err := st.Get(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.DeletedAt)
}
