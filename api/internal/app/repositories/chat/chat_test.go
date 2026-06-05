package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(
	t *testing.T,
) (context.Context, chat.Chat) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := chat.New(ax, db, func(domain.Chat) {})
	require.NoError(t, err)
	return context.Background(), repo
}

func TestChat_Create_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	created, err := repo.Create(ctx, "c1", "w1", "hello", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, created.Status)
	reloaded, err := repo.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", reloaded.WsID)
}

func TestChat_ResetIdle_Idempotent(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c1", "w1", "hello", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	second, err := repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, second.Status)
}

func TestChat_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c2", "w1", "hello", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Create(ctx, "c2", "w1", "hello", time.Unix(1, 0))
	assert.Error(t, err)
}

func TestChat_Get_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Get(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestChat_ResetIdle_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.ResetIdle(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestChat_Get_ReturnsCreated(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(500, 0)
	_, err := repo.Create(ctx, "c3", "w2", "hello", now)
	require.NoError(t, err)
	got, err := repo.Get(ctx, "c3")
	require.NoError(t, err)
	assert.Equal(t, "w2", got.WsID)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestChat_SetAgentRunning_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c1", "w1", "hello", time.Unix(1, 0))
	require.NoError(t, err)
	running, err := repo.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusAgentRunning, running.Status)
}

func TestChat_SetAgentRunning_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.SetAgentRunning(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestChat_ForkRenameDeleteList(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "c1", "w1", "root", now)
	require.NoError(t, err)
	forked, err := repo.Fork(ctx, "c2", "w1", "c1", "root (fork)", now)
	require.NoError(t, err)
	assert.Equal(t, "c1", forked.ParentID)
	renamed, err := repo.Rename(ctx, "c1", "renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Title)
	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	_, err = repo.Delete(ctx, "c2", now)
	require.NoError(t, err)
	list, err = repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestChat_List_ReflectsAll(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "c1", "w1", "a", now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, "c2", "w2", "b", now)
	require.NoError(t, err)
	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestChat_Fork_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Fork(ctx, "c2", "w1", "c1", "fork", now)
	require.NoError(t, err)
	_, err = repo.Fork(ctx, "c2", "w1", "c1", "fork again", now)
	assert.Error(t, err)
}

func TestChat_Rename_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Rename(ctx, "does-not-exist", "new title")
	assert.Error(t, err)
}

func TestChat_Delete_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Delete(ctx, "does-not-exist", time.Unix(1, 0))
	assert.Error(t, err)
}

func TestChat_New_ErrorOnClosedDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, dbErr := storesqlite.OpenDB(":memory:")
	require.NoError(t, dbErr)
	sqlDB, sqlErr := db.DB()
	require.NoError(t, sqlErr)
	sqlDB.Close()

	_, newErr := chat.New(ax, db, func(domain.Chat) {})
	assert.Error(t, newErr)
}
