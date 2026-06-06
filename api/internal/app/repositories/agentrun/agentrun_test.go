package agentrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(
	t *testing.T,
) (context.Context, agentrun.AgentRun) {
	t.Helper()
	ctx, repo, _ := newRepoWithDB(t)
	return ctx, repo
}

func newRepoWithDB(
	t *testing.T,
) (context.Context, agentrun.AgentRun, *gormdb.DB) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := agentrun.New(ax, db, func(domain.AgentRun) {})
	require.NoError(t, err)
	return context.Background(), repo, db
}

func TestAgentRun_Lifecycle_PendingRunningDone(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	done, err := repo.Complete(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusDone, done.Status)
}

func TestAgentRun_Fail_FromRunning(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	failed, err := repo.Fail(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, failed.Status)
}

func TestAgentRun_MarkRunning_RejectedFromDone(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.Complete(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	assert.Error(t, err)
}

func TestAgentRun_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a2", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Create(ctx, "a2", "w1", "c1", time.Unix(1, 0))
	assert.Error(t, err)
}

func TestAgentRun_Get_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Get(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestAgentRun_Complete_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Complete(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestAgentRun_Fail_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Fail(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestAgentRun_Get_ReturnsCreated(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "a3", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "a3")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusPending, got.Status)
	assert.Equal(t, "w1", got.WsID)
}

func TestAgentRun_ListRunning(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "a1", "w1", "c1", now)
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.Create(ctx, "a2", "w1", "c2", now)
	require.NoError(t, err)
	running, err := repo.ListRunning(ctx)
	require.NoError(t, err)
	require.Len(t, running, 1)
	assert.Equal(t, "a1", running[0].ID)
}

func TestAgentRun_ListByChat(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "a1", "w1", "c1", now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, "a2", "w1", "c2", now)
	require.NoError(t, err)
	list, err := repo.ListByChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "a1", list[0].ID)
}

func TestAgentRun_List(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "a1", "w1", "c1", now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, "a2", "w1", "c2", now)
	require.NoError(t, err)
	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestAgentRun_New_StoreError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = agentrun.New(ax, db, func(domain.AgentRun) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrun: store")
}

func TestAgentRun_List_StorageError(t *testing.T) {
	ctx, repo, db := newRepoWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = repo.List(ctx)
	require.Error(t, err)
}

func TestAgentRun_ListRunning_StorageError(t *testing.T) {
	ctx, repo, db := newRepoWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = repo.ListRunning(ctx)
	require.Error(t, err)
}

func TestAgentRun_ListByChat_StorageError(t *testing.T) {
	ctx, repo, db := newRepoWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = repo.ListByChat(ctx, "c1")
	require.Error(t, err)
}
