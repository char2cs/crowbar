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

func newStoreWithDeps(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat], *gormdb.DB) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(store.ChatEvent) {})
	require.NoError(t, err)
	return context.Background(), st, ax, db
}

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat]) {
	t.Helper()
	ctx, st, ax, _ := newStoreWithDeps(t)
	return ctx, st, ax
}

// openChat drives a chat through Create -> StartTurn -> SetTitle so the read model
// holds a chat with real folded state to assert on. There is no segment lifecycle to
// drive any more: a chat holds no process state at all (AgentSegment is deleted), so
// the multi-event history a projection test needs is now turn state and a title.
func openChat(
	t *testing.T,
	ctx context.Context,
	ax asynx.Asynx[domain.Chat],
	chatID string,
) {
	t.Helper()
	_, err := ax.SendWait(ctx, acCmds.Create{
		ID: chatID, WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, acCmds.StartTurn{ChatID: chatID, Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, acCmds.SetTitle{ChatID: chatID, Title: "title-" + chatID, Source: "user"})
	require.NoError(t, err)
}

func TestStore_GetChatReflectsFoldedState(t *testing.T) {
	ctx, st, ax := newStore(t)
	openChat(t, ctx, ax, "c1")

	got, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.WorkspaceID)
	assert.Equal(t, "title-c1", got.Title)
	assert.True(t, got.TitleLocked)
	assert.True(t, got.Working, "the read model reflects every folded event, not just the last")
	require.NotNil(t, got.CurrentTurnStarted)
}

func TestStore_GetChatMissingReturnsErrNotFound(t *testing.T) {
	ctx, st, _ := newStore(t)
	_, err := st.GetChat(ctx, "nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// TestStore_ListByWorkspace_ScopesToWorkspace proves ListByWorkspace returns
// every chat anchored to the queried workspace and excludes chats in other
// workspaces: c1+c2 (both w1) are returned, c3 (w2) is not.
func TestStore_ListByWorkspace_ScopesToWorkspace(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, acCmds.Create{ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c2", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	// c3 belongs to a DIFFERENT workspace — must never be returned for w1.
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c3", WorkspaceID: "w2", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	ids := make([]string, 0, len(list))
	for _, chat := range list {
		ids = append(ids, chat.ID)
	}
	assert.ElementsMatch(t, []string{"c1", "c2"}, ids, "both w1 chats must be returned; the w2 chat excluded")
}

// TestStore_ReadRebuildsWhenModelEmptyButLogNonEmpty proves the lazy Replay
// repair: after the durable read model is lost but the event log survives, a
// read enumerates every aggregate id via the event store's AggregateLister and
// Replays each back into state/store/agent_chat.db.
func TestStore_ReadRebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openChat(t, ctx, ax, "c1")

	got, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "title-c1", got.Title, "read model populated by the live store projection")

	// Simulate read-model loss with the event log intact.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	// GetChat heals the model via whole-model lazy Replay before concluding "not
	// found", so it must still resolve the chat with its whole folded history.
	rebuilt, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "title-c1", rebuilt.Title, "GetChat must Replay the id back from the event log")
	assert.True(t, rebuilt.Working)
}

// TestStore_GetChat_SelfHealsOnPerIDMiss proves the keyed GetChat's
// rebuild-on-miss fallback works even when the read model is NOT empty: only
// c2's row is dropped while c1's survives, so allHealed's empty-model rebuild
// would never fire — GetChat("c2") must still Replay c2 back from the event log
// on the per-id miss.
func TestStore_GetChat_SelfHealsOnPerIDMiss(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openChat(t, ctx, ax, "c1")
	openChat(t, ctx, ax, "c2")

	// Drop only c2's row; the model stays non-empty (c1 remains).
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read WHERE id = ?", "c2").Error)

	got, err := st.GetChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c2", got.ID)
	assert.Equal(t, "title-c2", got.Title, "GetChat must Replay c2 back on the per-id miss")
}

func TestStore_ListChats_EmptyLogReturnsEmpty(t *testing.T) {
	ctx, st, _ := newStore(t)
	rows, err := st.ListChats(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}

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
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, &fakeReplayES{err: errors.New("boom")}, ax, func(store.ChatEvent) {})
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
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	ctx := context.Background()
	openChat(t, ctx, ax, "c1")
	openChat(t, ctx, ax, "c2")

	// A third aggregate whose stored bytes were never written by a domain.Chat
	// command — stands in for any unreadable/corrupt entry in the shared event
	// log (asynx's own InternalEvent envelope, not a raw domain.Chat blob, so
	// this must fail json.Unmarshal on that envelope, not on domain.Chat itself).
	require.NoError(t, es.Append(ctx, "events:poison", 1, []byte("{not valid json")))

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	list, err := st.ListChats(ctx)
	require.NoError(t, err, "one unreplayable aggregate must not fail the whole rebuild")
	ids := make([]string, 0, len(list))
	for _, chat := range list {
		ids = append(ids, chat.ID)
	}
	assert.ElementsMatch(t, []string{"c1", "c2"}, ids)
}

func TestNew_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, nil, func(store.ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store")
}

// TestStore_GetChat_EmptyIDIsNotFound pins the guard ahead of the miss path:
// an empty id is NOWHERE, never a chat, and it must be rejected WITHOUT
// reaching healOne — healOne heals by Replaying the entire event log, and an
// agent runner that has left its chat carries an empty chat id on every hook,
// so an unguarded lookup would replay the whole log on every hook of every
// dying CLI.
func TestStore_GetChat_EmptyIDIsNotFound(t *testing.T) {
	ctx, st, _ := newStore(t)
	_, err := st.GetChat(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// TestStore_GetChat_HealFailsOnCorruptEvent proves a per-id miss whose heal
// attempt itself fails (rather than simply finding nothing) is reported as
// that failure, not folded into an ordinary ErrNotFound: the event log holds
// bytes for "poison" that were never written by a domain.Chat command (asynx's
// own InternalEvent envelope, not a raw domain.Chat blob — mirrors the
// existing whole-model TestStore_Rebuild_SkipsUnreplayableAggregate fixture,
// but exercised through the single-id healOne path GetChat takes on a miss).
func TestStore_GetChat_HealFailsOnCorruptEvent(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, es.Append(ctx, "events:poison", 1, []byte("{not valid json")))

	_, err = st.GetChat(ctx, "poison")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heal", "the failure must be attributed to the heal attempt")
	assert.NotErrorIs(t, err, store.ErrNotFound,
		"a heal that FAILED is not the same fact as a chat that genuinely never existed")
}

// TestStore_ListByWorkspace_PropagatesReadFailure mirrors
// TestStore_GetChat's own read-failure test at the ListByWorkspace/allHealed
// layer: a read-model that cannot be queried must surface as an error from
// both ListByWorkspace and the ListChats/allHealed call it makes internally,
// never silently reported as "this workspace has no chats".
func TestStore_ListByWorkspace_PropagatesReadFailure(t *testing.T) {
	ctx, st, _, db := newStoreWithDeps(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListByWorkspace(ctx, "w1")
	require.Error(t, err)
}

// minimalStore is an asynxModels.Store that implements none of the OPTIONAL
// serialize.AggregateLister capability — standing in for any event-store
// adapter that cannot enumerate its own aggregate ids.
type minimalStore struct{}

func (minimalStore) Append(context.Context, string, int64, []byte) error { return nil }
func (minimalStore) ReadFrom(context.Context, string, int64) ([][]byte, error) {
	return nil, nil
}

func (minimalStore) ReadRange(context.Context, string, int64, int64) ([][]byte, error) {
	return nil, nil
}
func (minimalStore) Delete(context.Context, string) error { return nil }

// TestStore_Rebuild_NoOpWhenEventStoreCannotEnumerate proves rebuild degrades
// to a harmless no-op — an empty list, no error — rather than panicking on a
// failed type assertion, when the underlying event store adapter offers no
// serialize.AggregateLister capability at all.
func TestStore_Rebuild_NoOpWhenEventStoreCannotEnumerate(t *testing.T) {
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(minimalStore{}).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, minimalStore{}, ax, func(store.ChatEvent) {})
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

	st, err := store.New(db, extraKeyLister{Store: es, extra: "snapshots:ghost"}, ax, func(store.ChatEvent) {})
	require.NoError(t, err)

	list, err := st.ListChats(ctx)
	require.NoError(t, err, "the bogus non-events key must be skipped, not fail the whole rebuild")
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"c1"}, ids, "the real aggregate must still heal despite the bogus key beside it")
}
