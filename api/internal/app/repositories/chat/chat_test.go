package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newRepo(t *testing.T) (context.Context, chat.Chat) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), chat.New(ax)
}

func TestChat_Create_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	created, err := repo.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, created.Status)
	reloaded, err := repo.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", reloaded.WsID)
}

func TestChat_ResetIdle_Idempotent(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	second, err := repo.ResetIdle(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, second.Status)
}
