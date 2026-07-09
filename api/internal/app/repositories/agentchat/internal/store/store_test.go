package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
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
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, es, ax, func(domain.AgentChat) {})
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

// openTwoSegmentChat drives a chat through Create -> EndSegment -> OpenSegment ->
// BindSession so the read model ends up with two segments (one ended, one active
// and bound to a provider session) — OpenSegment's Validate rejects a second
// active segment, so the first must be ended before the second opens.
func openTwoSegmentChat(
	t *testing.T,
	ctx context.Context,
	ax asynx.Asynx[domain.AgentChat],
	chatID string,
) {
	t.Helper()
	_, err := ax.SendWait(ctx, acCmds.Create{
		ID: chatID, WorkspaceID: "w1", SegmentID: "s1", CrowbarSegmentID: "cs1",
		ProviderID: "claude", TerminalSession: "term-1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, acCmds.EndSegment{ChatID: chatID, Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, acCmds.OpenSegment{
		ChatID: chatID, SegmentID: "s2", CrowbarSegmentID: "cs2",
		ProviderID: "codex", TerminalSession: "term-2", Now: time.Unix(3, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = ax.SendWait(ctx, acCmds.BindSession{
		ChatID: chatID, CrowbarSegmentID: "cs2", ProviderSessionID: "sess-" + chatID,
	})
	require.NoError(t, err)
}

func TestStore_GetChatReflectsBothSegments(t *testing.T) {
	ctx, st, ax := newStore(t)
	openTwoSegmentChat(t, ctx, ax, "c1")

	got, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got.Segments, 2)
	assert.Equal(t, "s1", got.Segments[0].ID)
	assert.Equal(t, "ended", got.Segments[0].Status)
	assert.Equal(t, "s2", got.Segments[1].ID)
	assert.Equal(t, "active", got.Segments[1].Status)
	assert.Equal(t, "sess-c1", got.Segments[1].ProviderSessionID)
}

func TestStore_GetChatMissingReturnsErrNotFound(t *testing.T) {
	ctx, st, _ := newStore(t)
	_, err := st.GetChat(ctx, "nope")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestStore_GetByProviderSession_FindsBoundChat(t *testing.T) {
	ctx, st, ax := newStore(t)
	openTwoSegmentChat(t, ctx, ax, "c1")

	got, err := st.GetByProviderSession(ctx, "sess-c1")
	require.NoError(t, err)
	assert.Equal(t, "c1", got.ID)
}

func TestStore_GetByProviderSession_MissingReturnsErrNotFound(t *testing.T) {
	ctx, st, ax := newStore(t)
	openTwoSegmentChat(t, ctx, ax, "c1")

	_, err := st.GetByProviderSession(ctx, "no-such-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestStore_ListChats_ExcludesDeleted(t *testing.T) {
	ctx, st, ax := newStore(t)
	openTwoSegmentChat(t, ctx, ax, "c1")
	openTwoSegmentChat(t, ctx, ax, "c2")

	_, err := ax.SendWait(ctx, acCmds.Delete{ChatID: "c2"})
	require.NoError(t, err)

	all, err := st.ListChats(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "c1", all[0].ID)

	// The deleted chat is still reachable by direct GetChat (tombstone, not gone).
	deleted, err := st.GetChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentChatStatusDeleted, deleted.Status)
}

func TestStore_ListByWorkspace_FiltersByWorkspaceAndExcludesDeleted(t *testing.T) {
	ctx, st, ax := newStore(t)
	_, err := ax.SendWait(ctx, acCmds.Create{
		ID: "c1", WorkspaceID: "w1", SegmentID: "s1", CrowbarSegmentID: "cs1",
		ProviderID: "claude", TerminalSession: "term-1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, acCmds.Create{
		ID: "c2", WorkspaceID: "w2", SegmentID: "s2", CrowbarSegmentID: "cs2",
		ProviderID: "claude", TerminalSession: "term-2", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	list, err := st.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)
}

// TestStore_ReadRebuildsWhenModelEmptyButLogNonEmpty proves the lazy Replay
// repair: after the durable read model is lost but the event log survives, a
// read enumerates every aggregate id via the event store's AggregateLister and
// Replays each back into state/store/agent_chat.db.
func TestStore_ReadRebuildsWhenModelEmptyButLogNonEmpty(t *testing.T) {
	ctx, st, ax, db := newStoreWithDeps(t)
	openTwoSegmentChat(t, ctx, ax, "c1")

	got, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got.Segments, 2, "read model populated by the live store projection")

	// Simulate read-model loss with the event log intact.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM agent_chats_read").Error)

	// GetChat heals the model via whole-model lazy Replay before concluding "not
	// found", so it must still resolve the chat correctly (both segments, bound
	// session).
	rebuilt, err := st.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, rebuilt.Segments, 2, "GetChat must Replay the id back from the event log")
	assert.Equal(t, "sess-c1", rebuilt.Segments[1].ProviderSessionID)

	byCleanSession, err := st.GetByProviderSession(ctx, "sess-c1")
	require.NoError(t, err)
	assert.Equal(t, "c1", byCleanSession.ID)
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
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, &fakeReplayES{err: errors.New("boom")}, ax, func(domain.AgentChat) {})
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

	_, err = store.New(db, nil, nil, func(domain.AgentChat) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store")
}
