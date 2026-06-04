package reviewthread_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, reviewthread.ReviewThread) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), reviewthread.New(ax)
}

func TestReviewThread_OpenResolveReopen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, "t1", "w1", time.Unix(1, 0))
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
	_, err := repo.Open(ctx, "t2", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Open(ctx, "t2", "w1", time.Unix(1, 0))
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
	_, err := repo.Open(ctx, "t3", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	assert.Error(t, err)
}

func TestReviewThread_Reopen_ErrorWhenOpen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, "t4", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Reopen(ctx, "t4")
	assert.Error(t, err)
}

func TestReviewThread_Get_ReturnsOpened(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, "t5", "w2", time.Unix(1, 0))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "t5")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusOpen, got.Status)
	assert.Equal(t, "w2", got.WsID)
}
