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
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestStore_ListReflectsProjection(t *testing.T) {
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

	st, err := store.New(context.Background(), db, ax, func(context.Context, domain.Workspace) {}, "")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

// TestStore_ReconcilesReadModelFromEventStoreOnOpen proves H3: if a projection
// is dropped (a crash between the durable event Append and the async projection,
// or saveWithRetry giving up), the read model is missing the row while the event
// store has it. Opening a fresh read model over that event store must reconcile —
// otherwise List stays permanently wrong while Get is correct.
func TestStore_ReconcilesReadModelFromEventStoreOnOpen(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ctx := context.Background()
	db1, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	_, err = store.New(ctx, db1, ax, func(context.Context, domain.Workspace) {}, "")
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	// A FRESH, empty read model over the SAME event store — the read model a
	// daemon would see after a crash dropped w1's projection. The projection bus
	// of this new store never saw w1's create, so only reconcile-on-open can
	// recover the row.
	db2, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax2, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax2.Shutdown(context.Background()) })

	st2, err := store.New(ctx, db2, ax2, func(context.Context, domain.Workspace) {}, "w1")
	require.NoError(t, err)

	all, err := st2.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1, "reconcile-on-open must heal the read model from the event store after a dropped projection")
	assert.Equal(t, "w1", all[0].ID)
	assert.Equal(t, "main", all[0].Branch)
}

func TestStore_GetReflectsProjection(t *testing.T) {
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

	st, err := store.New(context.Background(), db, ax, func(context.Context, domain.Workspace) {}, "")
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	got, err := st.Get(ctx, "w2")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "main", got.Branch)
}

func TestStore_GetMissingReturnsNil(t *testing.T) {
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

	st, err := store.New(context.Background(), db, ax, func(context.Context, domain.Workspace) {}, "")
	require.NoError(t, err)

	got, err := st.Get(context.Background(), "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}
