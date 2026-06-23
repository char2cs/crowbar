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

	st, err := store.New(db, ax, func(context.Context, domain.Workspace) {})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
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

	st, err := store.New(db, ax, func(context.Context, domain.Workspace) {})
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

	st, err := store.New(db, ax, func(context.Context, domain.Workspace) {})
	require.NoError(t, err)

	got, err := st.Get(context.Background(), "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}
