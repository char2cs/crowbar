package store_test

import (
	"context"
	"errors"
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
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// bareEventStore satisfies asynxModels.Store (via embedding a nil one — its
// promoted methods are never called by anything under test) but deliberately
// does NOT implement serialize.AggregateLister, exercising rebuild's
// "event store can't enumerate" no-op branch.
type bareEventStore struct {
	asynxModels.Store
}

// listerEventStore satisfies both asynxModels.Store and serialize.AggregateLister,
// letting a test control exactly which raw keys rebuild sees without needing a
// real event store that happens to hold them.
type listerEventStore struct {
	asynxModels.Store
	ids []string
	err error
}

func (l *listerEventStore) AggregateIDs(context.Context) ([]string, error) {
	return l.ids, l.err
}

func newServiceOver(
	t *testing.T,
	es asynxModels.Store,
) (context.Context, store.Store, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	ctx := context.Background()
	realES, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(realES).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax)
	require.NoError(t, err)
	return ctx, st, ax
}

// TestListOrRebuild_SkipsRebuildWhenEventStoreLacksAggregateLister proves the
// best-effort contract stated on serialize.AggregateLister's own doc comment: an
// event store without the capability is a silent no-op, not an error, so a
// deployment on such a store degrades to "never heals" rather than crashing.
func TestListOrRebuild_SkipsRebuildWhenEventStoreLacksAggregateLister(t *testing.T) {
	ctx, st, _ := newServiceOver(t, &bareEventStore{})

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestListOrRebuild_ErrorWhenAggregateIDsFails(t *testing.T) {
	ctx, st, _ := newServiceOver(t, &listerEventStore{err: errors.New("event log unavailable")})

	_, err := st.ListOrRebuild(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}

// TestListOrRebuild_SkipsNonEventKeys proves rebuild only replays "events:"
// keys: the event store's raw key space also holds "snapshots:<id>" rows for the
// same aggregate, and replaying those as if they were an aggregate id would
// either error or double-replay.
func TestListOrRebuild_SkipsNonEventKeys(t *testing.T) {
	ctx, st, _ := newServiceOver(t, &listerEventStore{ids: []string{"snapshots:w1"}})

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err, "a non-event key must be skipped, never replayed")
	assert.Empty(t, rows)
}

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
		WithSnapshotStore(asynxstore.NewSnapshots()).
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
		WithSnapshotStore(asynxstore.NewSnapshots()).
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

// TestListOrRebuild_ReplayErrorAbortsTheWholeRebuild proves rebuild does NOT
// tolerate one corrupt aggregate the way a sibling journal's own rebuild does
// (chat/internal/store skips an unreplayable entry and heals everything else):
// here a single "events:" key that Replay cannot fold aborts the entire heal, so
// a caller sees the failure rather than a silently incomplete read model.
func TestListOrRebuild_ReplayErrorAbortsTheWholeRebuild(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax)
	require.NoError(t, err)

	// A "poison" aggregate whose stored bytes were never written by a domain.Workspace
	// command — asynx's own envelope, not a raw domain.Workspace blob, so Replay must
	// fail decoding it rather than folding garbage into the read model.
	require.NoError(t, es.Append(ctx, "events:poison", 1, []byte("{not valid json")))

	_, err = st.ListOrRebuild(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "replay poison")
}

// TestListOrRebuild_FoldErrorDuringReplayIsLoggedNotSurfaced proves foldReplayed
// treats a read-model persistence failure as best-effort logging, not a hard
// error: List's own SELECT still works against a read-only connection, only the
// write inside the replay's fold callback fails, and ListOrRebuild must still
// succeed with the row simply absent rather than surfacing that internal error.
func TestListOrRebuild_FoldErrorDuringReplayIsLoggedNotSurfaced(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax)
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	// Lose the durable row, then make the connection read-only: List's SELECT
	// still succeeds (empty), but the replay's fold can no longer persist the
	// healed row back.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_workspaces").Error)
	require.NoError(t, db.WithContext(ctx).Exec("PRAGMA query_only = ON").Error)

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err, "a Fold failure during replay must be logged, not surfaced")
	assert.Empty(t, rows, "the row could not be persisted back, so it is still absent")
}
