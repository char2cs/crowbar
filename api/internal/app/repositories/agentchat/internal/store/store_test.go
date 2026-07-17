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
	acCmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStoreWithDeps(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.AgentChat], *gormdb.DB) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(string, string, string, bool) {})
	require.NoError(t, err)
	return context.Background(), st, ax, db
}

func newStore(
	t *testing.T,
) (context.Context, store.Store, asynx.Asynx[domain.AgentChat]) {
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
	ax asynx.Asynx[domain.AgentChat],
	chatID string,
) {
	t.Helper()
	_, err := ax.SendWait(ctx, acCmds.Create{
		ID: chatID, WorkspaceID: "w1", Now: time.Unix(1, 0).UTC(),
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
	_, err := ax.SendWait(ctx, acCmds.Create{ID: "c1", WorkspaceID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c2", WorkspaceID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	// c3 belongs to a DIFFERENT workspace — must never be returned for w1.
	_, err = ax.SendWait(ctx, acCmds.Create{ID: "c3", WorkspaceID: "w2", Now: time.Unix(1, 0).UTC()})
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
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, &fakeReplayES{err: errors.New("boom")}, ax, func(string, string, string, bool) {})
	require.NoError(t, err)

	_, err = st.ListChats(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enumerate aggregate ids")
}

func TestNew_StorageError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, nil, func(string, string, string, bool) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store")
}
