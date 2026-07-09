package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestListOrRebuild_RebuildsWhenModelEmptyButLogNonEmpty proves the lazy Replay
// repair (spec §3.7, decision 7): after the durable read model is lost but the
// event log survives, the per-request ListOrRebuild enumerates every aggregate id
// via the event store's AggregateLister and Replays each back into
// state/store/workspace.db — while the RAW List (the boot orphan-sweep path)
// deliberately does NOT replay, so boot pays nothing.
func TestListOrRebuild_RebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax)
	require.NoError(t, err)

	// Seed two workspaces. SendWait makes the store projection deterministic.
	for _, id := range []string{"w1", "w2"} {
		_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
			ID: id, RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		})
		require.NoError(t, err)
	}

	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2, "read model populated by the live store projection")

	// Simulate read-model loss with the event log intact: wipe the durable rows.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_workspaces").Error)

	// RAW List reflects the loss AND does NOT auto-rebuild (the boot-sweep path):
	// this is the "boot (no ListOrRebuild) did NOT replay" assertion.
	raw, err := st.List(ctx)
	require.NoError(t, err)
	require.Empty(t, raw, "raw List must not trigger lazy Replay")

	// The per-request path heals the model via whole-model lazy Replay.
	rebuilt, err := st.ListOrRebuild(ctx)
	require.NoError(t, err)
	require.Len(t, rebuilt, 2, "ListOrRebuild must Replay every id from the event log")
	got := []string{rebuilt[0].ID, rebuilt[1].ID}
	require.ElementsMatch(t, []string{"w1", "w2"}, got)
	require.Equal(t, "p1", rebuilt[0].ProjectID)

	// The healed rows are now durable, so the raw List sees them too.
	healed, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, healed, 2)
}

// TestListOrRebuild_EmptyLogReturnsEmpty proves that with a genuinely empty event
// log there is nothing to Replay: ListOrRebuild returns an empty list without
// error (the empty-model / empty-log case, not the repair trigger).
func TestListOrRebuild_EmptyLogReturnsEmpty(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax)
	require.NoError(t, err)

	rows, err := st.ListOrRebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}
