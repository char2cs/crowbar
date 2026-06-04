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
