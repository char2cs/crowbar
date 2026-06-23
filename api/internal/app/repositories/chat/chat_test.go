package chat_test

import (
	"context"
	"fmt"
	"sync"
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
	repo, err := chat.New(context.Background(), ax, es, db, func(domain.Chat) {})
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

// TestChat_ConcurrentSameAggregateCommandsAllSucceed proves R6 for the global
// chat repo: many commands targeting the SAME chat aggregate at once must ALL
// commit. The chat aggregate shares one asynx (8 workers) and one event store
// across every chat; without single-writer-per-aggregate serialization the
// optimistic event store rejects the losers with a version/PK conflict and the
// read model silently goes stale.
func TestChat_ConcurrentSameAggregateCommandsAllSucceed(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, "c1", "w1", "root", now)
	require.NoError(t, err)

	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.Rename(ctx, "c1", fmt.Sprintf("title-%d", idx))
		}(i)
	}
	wg.Wait()

	failed := 0
	for _, e := range errs {
		if e != nil {
			failed++
		}
	}
	require.Zero(t, failed, "%d/%d concurrent same-aggregate commands failed with a version conflict", failed, n)
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

	_, newErr := chat.New(context.Background(), ax, es, db, func(domain.Chat) {})
	assert.Error(t, newErr)
}
