package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type bareEventStore struct {
	asynxModels.Store
}

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
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	pst, err := projections.NewStore(db)
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(realES).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	projections.Subscriber(pst)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	st := store.New(pst, es, ax)
	return ctx, st, ax
}

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

func TestListOrRebuild_SkipsNonEventKeys(t *testing.T) {
	ctx, st, _ := newServiceOver(t, &listerEventStore{ids: []string{"snapshots:w1"}})

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err, "a non-event key must be skipped, never replayed")
	assert.Empty(t, rows)
}

func TestListOrRebuild_RebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	pst, err := projections.NewStore(db)
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	projections.Subscriber(pst)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	st := store.New(pst, es, ax)

	for _, id := range []string{"w1", "w2"} {
		_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
			ID: id, RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
			Provisioning: domain.WorkspacePlaceholder,
		})
		require.NoError(t, err)
	}

	all, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2, "read model populated by the live store projection")

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_workspaces").Error)

	raw, err := st.List(ctx)
	require.NoError(t, err)
	require.Empty(t, raw, "raw List must not trigger lazy Replay")

	rebuilt, err := st.ListOrRebuild(ctx)
	require.NoError(t, err)
	require.Len(t, rebuilt, 2, "ListOrRebuild must Replay every id from the event log")
	got := []string{rebuilt[0].ID, rebuilt[1].ID}
	require.ElementsMatch(t, []string{"w1", "w2"}, got)
	require.Equal(t, "p1", rebuilt[0].ProjectID)

	healed, err := st.List(ctx)
	require.NoError(t, err)
	require.Len(t, healed, 2)
}

func TestListOrRebuild_EmptyLogReturnsEmpty(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	pst, err := projections.NewStore(db)
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	projections.Subscriber(pst)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	st := store.New(pst, es, ax)

	rows, err := st.ListOrRebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestListOrRebuild_ReplayErrorAbortsTheWholeRebuild(t *testing.T) {
	ctx := context.Background()

	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	pst, err := projections.NewStore(db)
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	projections.Subscriber(pst)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })

	st := store.New(pst, es, ax)

	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error {
		return tx.Append(ctx, "events:poison", 1, []byte("{not valid json"))
	}))

	_, err = st.ListOrRebuild(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "replay poison")
}

func TestListOrRebuild_AFoldErrorDuringReplayIsReturned(t *testing.T) {
	ctx, st, ax, _, db := newSharedLogService(t)

	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_workspaces").Error)
	require.NoError(t, db.WithContext(ctx).Exec("PRAGMA query_only = ON").Error)

	_, err = st.ListOrRebuild(ctx)

	require.Error(t, err)
}

func TestListOrRebuild_NeverResurrectsATombstonedWorkspace(t *testing.T) {
	ctx, st, ax, es, db := newSharedLogService(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)
	appendTombstone(t, es, "w1")
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_workspaces").Error)

	rows, err := st.ListOrRebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func newSharedLogService(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Workspace], asynxModels.Store, *gormdb.DB) {
	t.Helper()
	ctx := context.Background()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	pst, err := projections.NewStore(db)
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	projections.Subscriber(pst)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(ctx) })
	return ctx, store.New(pst, es, ax), ax, es, db
}

func appendTombstone(t *testing.T, es asynxModels.Store, id string) {
	t.Helper()
	ctx := context.Background()
	prior, err := es.ReadFrom(ctx, "events:"+id, 1)
	require.NoError(t, err)
	var last struct {
		SchemaVersion int `json:"schema_version"`
	}
	require.NoError(t, json.Unmarshal(prior[len(prior)-1], &last))
	v := int64(len(prior) + 1)
	evt, err := json.Marshal(map[string]any{
		"id": "tomb-" + id, "event_name": asynxModels.ForgetEventName, "version": v,
		"schema_version": last.SchemaVersion, "occurred_at": time.Unix(2, 0).UTC(), "patches": json.RawMessage("[]"),
	})
	require.NoError(t, err)
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error { return tx.Append(ctx, "events:"+id, v, evt) }))
}

func TestListOrRebuild_AnswersARowThatWillNotDecodeFromItsHistory(t *testing.T) {
	ctx, st, ax, _, db := newSharedLogService(t)
	for _, id := range []string{"w1", "w2"} {
		_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
			ID: id, RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
			Provisioning: domain.WorkspacePlaceholder,
		})
		require.NoError(t, err)
	}
	require.NoError(t, db.WithContext(ctx).Exec("UPDATE read_workspaces SET data = ? WHERE id = 'w1'", []byte("{not json")).Error)

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	byID := map[string]domain.Workspace{rows[0].ID: rows[0], rows[1].ID: rows[1]}
	assert.Equal(t, "r1", byID["w1"].RepoID, "the undecodable row is read from its history")
	raw, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, raw, 1)
}

func TestListOrRebuild_LeavesOutARowThatWillNotDecodeAndHasNoHistory(t *testing.T) {
	ctx, st, ax, _, db := newSharedLogService(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)
	require.NoError(t, db.WithContext(ctx).Exec("INSERT INTO read_workspaces (id, data) VALUES ('ghost', ?)", []byte("{not json")).Error)

	rows, err := st.ListOrRebuild(ctx)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestListOrRebuild_FailsWhenARowAndItsHistoryWillNotRead(t *testing.T) {
	ctx, st, _, es, db := newSharedLogService(t)
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error {
		return tx.Append(ctx, "events:poison", 1, []byte("{not valid json"))
	}))
	require.NoError(t, db.WithContext(ctx).Exec("INSERT INTO read_workspaces (id, data) VALUES ('poison', ?)", []byte("{not json")).Error)

	_, err := st.ListOrRebuild(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "row poison will not decode")
}
