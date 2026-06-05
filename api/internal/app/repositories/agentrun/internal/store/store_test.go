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
	arcmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.AgentRun]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, ax, func(domain.AgentRun) {})
	require.NoError(t, err)
	return context.Background(), st, ax
}

func TestStore_ListReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{
		ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestStore_GetReflectsProjection(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{
		ID: "a2", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	got, err := st.Get(ctx, "a2")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.AgentRunStatusPending, got.Status)
}

func TestStore_GetMissingReturnsNil(t *testing.T) {
	ctx, st, _ := newStore(t)
	got, err := st.Get(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStore_ListRunning_FiltersByStatus(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, arcmds.MarkAgentRunRunning{ID: "a1"})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a2", WsID: "w1", ChatID: "c2", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	running, err := st.ListRunning(ctx)
	require.NoError(t, err)
	assert.Len(t, running, 1)
	assert.Equal(t, "a1", running[0].ID)
}

func TestStore_ListRunning_NonMatchReturnsEmpty(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	running, err := st.ListRunning(ctx)
	require.NoError(t, err)
	assert.Empty(t, running)
}

func TestStore_ListByChat_FiltersByChatID(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a2", WsID: "w1", ChatID: "c2", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	list, err := st.ListByChat(ctx, "c1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "a1", list[0].ID)
}

func TestStore_ListByChat_NonMatchReturnsEmpty(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	list, err := st.ListByChat(ctx, "c-no-match")
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestStore_ListRunning_StorageError verifies that storage errors propagate.
func TestStore_ListRunning_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(db, ax, func(domain.AgentRun) {})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListRunning(context.Background())
	require.Error(t, err)
}

// TestStore_ListByChat_StorageError verifies that storage errors propagate.
func TestStore_ListByChat_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(db, ax, func(domain.AgentRun) {})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListByChat(context.Background(), "c1")
	require.Error(t, err)
}
