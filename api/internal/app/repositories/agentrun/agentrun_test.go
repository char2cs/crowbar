package agentrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, agentrun.AgentRun) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), agentrun.New(ax)
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
