package store_test

import (
	"context"
	"encoding/json"
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
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStoreWithDeps(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat], *gormdb.DB) {
	t.Helper()
	ctx, st, ax, db, _ := newStoreWithLog(t)
	return ctx, st, ax, db
}

func newStoreWithLog(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat], *gormdb.DB, asynxModels.Store) {
	t.Helper()
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
	return context.Background(), st, ax, db, es
}

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.Chat]) {
	t.Helper()
	ctx, st, ax, _ := newStoreWithDeps(t)
	return ctx, st, ax
}

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

	_, err = ax.SendWait(ctx, acCmds.SetSelection{ChatID: chatID, Model: "m1", Effort: "high"})
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
	assert.Equal(t, "m1", got.Model, "the read model reflects every folded event, not just the last")
}

func TestStore_GetChatMissingReturnsErrNotFound(t *testing.T) {
	ctx, st, _ := newStore(t)
	_, err := st.GetChat(ctx, "nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestStore_ListByWorkspace_ScopesToWorkspace(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, acCmds.Create{ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c2", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
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

func TestStore_ReadRebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openChat(t, ctx, ax, "c1")

	got, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "title-c1", got.Title, "read model populated by the live store projection")

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	rebuilt, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "title-c1", rebuilt.Title, "GetChat must Replay the id back from the event log")
	assert.Equal(t, "m1", rebuilt.Model)
}

func TestStore_GetChat_SelfHealsOnPerIDMiss(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openChat(t, ctx, ax, "c1")
	openChat(t, ctx, ax, "c2")

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

func TestOpen_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.Open(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store")
}

func TestStore_GetChat_EmptyIDIsNotFound(t *testing.T) {
	ctx, st, _ := newStore(t)
	_, err := st.GetChat(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestStore_GetChat_HealFailsOnCorruptEvent(t *testing.T) {
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
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error {
		return tx.Append(ctx, "events:poison", 1, []byte("{not valid json"))
	}))

	_, err = st.GetChat(ctx, "poison")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heal", "the failure must be attributed to the heal attempt")
	assert.NotErrorIs(t, err, store.ErrNotFound,
		"a heal that FAILED is not the same fact as a chat that genuinely never existed")
}

func TestStore_ListByWorkspace_PropagatesReadFailure(t *testing.T) {
	ctx, st, _, db := newStoreWithDeps(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ListByWorkspace(ctx, "w1")
	require.Error(t, err)
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

func TestStore_AHealThatCannotSaveIsAnError(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openChat(t, ctx, ax, "c1")
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)
	require.NoError(t, db.WithContext(ctx).Exec("PRAGMA query_only = ON").Error)

	_, err := st.GetChat(ctx, "c1")
	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrNotFound, "a row that could not be saved is not a missing chat")
	_, err = st.ListChats(ctx)
	require.Error(t, err)
}

func TestStore_AHealNeverResurrectsATombstonedChat(t *testing.T) {
	ctx, st, ax, db, es := newStoreWithLog(t)
	openChat(t, ctx, ax, "c1")
	appendTombstone(t, es, "c1")
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	_, err := st.GetChat(ctx, "c1")
	require.ErrorIs(t, err, store.ErrNotFound)
	all, err := st.ListChats(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}
