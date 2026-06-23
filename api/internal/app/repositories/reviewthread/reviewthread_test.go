package reviewthread_test

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
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(
	t *testing.T,
) (context.Context, reviewthread.ReviewThread) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := reviewthread.New(context.Background(), ax, es, db, func(domain.ReviewThread) {})
	require.NoError(t, err)
	return context.Background(), repo
}

func TestReviewThread_OpenResolveReopen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1",
	}, time.Unix(1, 0))
	require.NoError(t, err)
	resolved, err := repo.Resolve(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusResolved, resolved.Status)
	reopened, err := repo.Reopen(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusOpen, reopened.Status)
}

func TestReviewThread_Open_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w1", MessageID: "m2"}, time.Unix(1, 0))
	assert.Error(t, err)
}

func TestReviewThread_Get_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Get(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestReviewThread_Resolve_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Resolve(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestReviewThread_Reopen_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Reopen(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestReviewThread_Resolve_ErrorWhenAlreadyResolved(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t3", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	assert.Error(t, err)
}

func TestReviewThread_Reopen_ErrorWhenOpen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t4", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Reopen(ctx, "t4")
	assert.Error(t, err)
}

func TestReviewThread_Get_ReturnsOpened(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t5", WsID: "w2", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "t5")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusOpen, got.Status)
	assert.Equal(t, "w2", got.WsID)
}

func TestReviewThread_OpenReplyList(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "b.go", Body: "first",
	}, now)
	require.NoError(t, err)

	replied, err := repo.Reply(ctx, "t1", "m2", "", false, "second", now)
	require.NoError(t, err)
	require.Len(t, replied.Messages, 2)
	assert.Equal(t, "second", replied.Messages[1].Body)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	byWs, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, byWs, 1)
	assert.Equal(t, "t1", byWs[0].ID)
}

func TestReviewThread_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w2", MessageID: "m2"}, now)
	require.NoError(t, err)

	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "t1", list[0].ID)

	none, err := repo.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestReviewThread_Reply_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Reply(ctx, "no-thread", "m1", "", false, "body", time.Unix(1, 0))
	assert.Error(t, err)
}

// TestReviewThread_ConcurrentSameAggregateCommandsAllSucceed proves R6 for the
// global reviewthread repo: many commands targeting the SAME thread aggregate at
// once must ALL commit. The reviewthread aggregate shares one asynx (8 workers)
// and one event store across every thread; without single-writer-per-aggregate
// serialization the optimistic event store rejects the losers with a version/PK
// conflict and the read model silently goes stale.
func TestReviewThread_ConcurrentSameAggregateCommandsAllSucceed(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m0", Body: "first",
	}, now)
	require.NoError(t, err)

	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.Reply(ctx, "t1", fmt.Sprintf("m-%d", idx), "", false, "reply", now)
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

func TestReviewThread_List_StorageError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := reviewthread.New(context.Background(), ax, es, db, func(domain.ReviewThread) {})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.List(context.Background())
	assert.Error(t, err)
}

func TestReviewThread_ListByWorkspace_StorageError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := reviewthread.New(context.Background(), ax, es, db, func(domain.ReviewThread) {})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.ListByWorkspace(context.Background(), "w1")
	assert.Error(t, err)
}

func TestReviewThread_New_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
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

	_, err = reviewthread.New(context.Background(), ax, es, db, func(domain.ReviewThread) {})
	assert.Error(t, err)
}
