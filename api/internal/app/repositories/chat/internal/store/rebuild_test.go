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
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	acCmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeReplayES is a serialize.AggregateLister whose AggregateIDs errors, so a
// read over an empty read model surfaces the rebuild's enumeration failure.
type fakeReplayES struct {
	asynxModels.Store
	err error
}

func (f *fakeReplayES) AggregateIDs(context.Context) ([]string, error) {
	return nil, f.err
}

func TestStore_Read_RebuildEnumerationError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	proj, err := store.Open(db)
	require.NoError(t, err)
	b := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	store.Subscriber(proj)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(proj, &fakeReplayES{err: errors.New("boom")}, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	_, err = st.ListChats(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}

// TestStore_Rebuild_SkipsUnreplayableAggregate proves rebuild does not abort
// the whole read model when ONE aggregate in the shared event log cannot be
// replayed into domain.Chat (a pre-cutover AgentChat payload, or any other
// corrupt/incompatible bytes) — c1 and c2 must still heal and be returned.
// Before this fix, rebuild returned the first Replay error and the caller got
// nothing back: every chat, not just the broken one.
func TestStore_Rebuild_SkipsUnreplayableAggregate(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	proj, err := store.Open(db)
	require.NoError(t, err)
	b := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	store.Subscriber(proj)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(proj, es, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	ctx := context.Background()
	openChat(t, ctx, ax, "c1")
	openChat(t, ctx, ax, "c2")

	// A third aggregate whose stored bytes were never written by a domain.Chat
	// command — stands in for any unreadable/corrupt entry in the shared event
	// log (asynx's own InternalEvent envelope, not a raw domain.Chat blob, so
	// this must fail json.Unmarshal on that envelope, not on domain.Chat itself).
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error {
		return tx.Append(ctx, "events:poison", 1, []byte("{not valid json"))
	}))

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	list, err := st.ListChats(ctx)
	require.NoError(t, err, "one unreplayable aggregate must not fail the whole rebuild")
	ids := make([]string, 0, len(list))
	for _, chat := range list {
		ids = append(ids, chat.ID)
	}
	assert.ElementsMatch(t, []string{"c1", "c2"}, ids)
}

// TestStore_Rebuild_NoOpWhenEventStoreCannotEnumerate proves rebuild degrades
// to a harmless no-op — an empty list, no error — rather than panicking on a
// failed type assertion, when the underlying event store adapter offers no
// serialize.AggregateLister capability at all.
func TestStore_Rebuild_NoOpWhenEventStoreCannotEnumerate(t *testing.T) {
	es := asynxstore.New() // the in-memory store cannot enumerate its aggregates
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	proj, err := store.Open(db)
	require.NoError(t, err)
	b := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	store.Subscriber(proj)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	st, err := store.New(proj, es, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	list, err := st.ListChats(context.Background())
	require.NoError(t, err, "an event store that cannot enumerate aggregates must degrade to an empty list, not error")
	assert.Empty(t, list)
}

// extraKeyLister wraps a real serialize.AggregateLister and injects one extra
// key into its answer that does NOT carry the "events:" prefix — standing in
// for a "snapshots:<id>" row the shared event log's raw key scan can return
// alongside the real "events:<id>" rows (see event_store.go's eventKeyPrefix
// doc).
type extraKeyLister struct {
	asynxModels.Store
	extra string
}

func (e extraKeyLister) AggregateIDs(ctx context.Context) ([]string, error) {
	lister, ok := e.Store.(serialize.AggregateLister)
	if !ok {
		return nil, errors.New("extraKeyLister: embedded store cannot enumerate")
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return nil, err
	}
	return append(keys, e.extra), nil
}

// TestStore_Rebuild_SkipsNonEventKey proves rebuild's CutPrefix guard: a raw
// key that is not an "events:<id>" row must be skipped rather than replayed
// as if the whole key were an aggregate id, while every genuine "events:<id>"
// key alongside it still heals normally.
func TestStore_Rebuild_SkipsNonEventKey(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ctx := context.Background()
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	proj, err := store.Open(db)
	require.NoError(t, err)
	st, err := store.New(proj, extraKeyLister{Store: es, extra: "snapshots:ghost"}, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	list, err := st.ListChats(ctx)
	require.NoError(t, err, "the bogus non-events key must be skipped, not fail the whole rebuild")
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"c1"}, ids, "the real aggregate must still heal despite the bogus key beside it")
}

// countingES counts the whole-model rebuilds a read starts: each one
// enumerates the log's aggregate ids first.
type countingES struct {
	asynxModels.Store
	lister       serialize.AggregateLister
	enumerations int
}

func (c *countingES) AggregateIDs(ctx context.Context) ([]string, error) {
	c.enumerations++
	return c.lister.AggregateIDs(ctx)
}

func newCountedStore(t *testing.T) (context.Context, store.Store, asynx.Asynx[domain.Chat], *gormdb.DB, *countingES) {
	t.Helper()
	ctx, _, ax, db, es := newStoreWithLog(t)
	lister, ok := es.(serialize.AggregateLister)
	require.True(t, ok)
	counted := &countingES{Store: es, lister: lister}
	proj, err := store.Open(db)
	require.NoError(t, err)
	st, err := store.New(proj, counted, ax, func(store.ChatEvent) {})
	require.NoError(t, err)
	return ctx, st, ax, db, counted
}

// A model whose every row will not decode is rebuilt from the log, which
// rewrites those rows, so the next read needs no rebuild.
func TestStore_AModelOfOnlyUndecodableRowsHealsOnce(t *testing.T) {
	ctx, st, ax, db, es := newCountedStore(t)
	openChat(t, ctx, ax, "c1")
	openChat(t, ctx, ax, "c2")
	require.NoError(t, db.WithContext(ctx).Exec("UPDATE agent_chats_read SET data = ?", []byte("{not json")).Error)

	healed, err := st.ListChats(ctx)
	require.NoError(t, err)
	again, err := st.ListChats(ctx)
	require.NoError(t, err)

	assert.Len(t, healed, 2, "the rebuild rewrites the undecodable rows")
	assert.Len(t, again, 2)
	assert.Equal(t, 1, es.enumerations)
}

// Rows a rebuild cannot heal are not rebuilt again on every read: only a
// changed row set earns another rebuild.
func TestStore_ARebuildThatHealsNothingIsNotRepeatedForTheSameRows(t *testing.T) {
	ctx, st, _, db, es := newCountedStore(t)
	require.NoError(t, db.WithContext(ctx).Exec("INSERT INTO agent_chats_read (id, workspace_id, data) VALUES ('ghost', 'w1', ?)", []byte("{not json")).Error)

	for range 3 {
		rows, err := st.ListChats(ctx)
		require.NoError(t, err)
		assert.Empty(t, rows)
	}
	assert.Equal(t, 1, es.enumerations, "the same undecodable rows are rebuilt once")

	require.NoError(t, db.WithContext(ctx).Exec("INSERT INTO agent_chats_read (id, workspace_id, data) VALUES ('ghost2', 'w1', ?)", []byte("{not json")).Error)
	_, err := st.ListChats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, es.enumerations, "a changed row set is rebuilt again")
}

func TestStore_AnEmptyModelThatRebuildLeavesEmptyIsNotRebuiltOnEveryRead(t *testing.T) {
	ctx, st, _, _, es := newCountedStore(t)

	for range 3 {
		_, err := st.ListChats(ctx)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, es.enumerations)
}

// A rebuild that healed rows does not stop a later lost model from healing.
func TestStore_AModelLostAfterAHealIsRebuiltAgain(t *testing.T) {
	ctx, st, ax, db, es := newCountedStore(t)
	openChat(t, ctx, ax, "c1")
	require.NoError(t, db.WithContext(ctx).Exec("UPDATE agent_chats_read SET data = ?", []byte("{not json")).Error)
	_, err := st.ListChats(ctx)
	require.NoError(t, err)
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	rows, err := st.ListChats(ctx)

	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, 2, es.enumerations)
}
