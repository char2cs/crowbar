package repositories_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type captureChatRepo struct {
	chat.Chat
	mu         sync.Mutex
	runningIDs []string
	resetIDs   []string
	runningErr error
	resetErr   error
}

func (c *captureChatRepo) SetAgentRunning(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runningIDs = append(c.runningIDs, id)
	return domain.Chat{}, c.runningErr
}

func (c *captureChatRepo) ResetIdle(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetIDs = append(c.resetIDs, id)
	return domain.Chat{}, c.resetErr
}

func (c *captureChatRepo) running(
	id string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.runningIDs {
		if got == id {
			return true
		}
	}
	return false
}

func (c *captureChatRepo) reset(
	id string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, got := range c.resetIDs {
		if got == id {
			return true
		}
	}
	return false
}

func newAgentRunAsynx(
	t *testing.T,
) (asynx.Asynx[domain.AgentRun], agentrun.AgentRun) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	runs, err := agentrun.New(ax, db, func(domain.AgentRun) {})
	require.NoError(t, err)
	return ax, runs
}

func TestAgentRunProjection_DrivesChatStatus(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{}
	var refreshed []string
	var mu sync.Mutex
	refresh := func(_ context.Context, wsID string) {
		mu.Lock()
		defer mu.Unlock()
		refreshed = append(refreshed, wsID)
	}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, refresh))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.running("c1") }, time.Second, 5*time.Millisecond)

	_, err = runs.Complete(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.reset("c1") }, time.Second, 5*time.Millisecond)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(refreshed) >= 1
	}, time.Second, 5*time.Millisecond)
}

func TestAgentRunProjection_ConcurrentRuns_ResetsOnlyWhenLastEnds(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, func(context.Context, string) {}))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = runs.Create(ctx, "a2", "w1", "c1", time.Unix(2, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a2")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := runs.ListByChat(ctx, "c1")
		return listErr == nil && countRunning(running) == 2
	}, time.Second, 5*time.Millisecond)

	_, err = runs.Complete(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := runs.ListByChat(ctx, "c1")
		return listErr == nil && countRunning(running) == 1
	}, time.Second, 5*time.Millisecond)
	assert.False(t, fakeChat.reset("c1"), "chat must stay agent-running while a2 is live")

	_, err = runs.Complete(ctx, "a2")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.reset("c1") }, time.Second, 5*time.Millisecond)
}

func countRunning(
	runs []domain.AgentRun,
) int {
	n := 0
	for _, r := range runs {
		if r.Status == domain.AgentRunStatusRunning {
			n++
		}
	}
	return n
}

type listByChatErrRunRepo struct {
	agentrun.AgentRun
}

func (listByChatErrRunRepo) ListByChat(
	_ context.Context,
	_ string,
) ([]domain.AgentRun, error) {
	return nil, errFake
}

func TestAgentRunProjection_ListByChatErrorResetsAnyway(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, listByChatErrRunRepo{}, func(context.Context, string) {}))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = runs.Complete(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.reset("c1") }, time.Second, 5*time.Millisecond)
}

func TestAgentRunProjection_TerminalErrorAndInterrupted(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, func(context.Context, string) {}))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = runs.Fail(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.reset("c1") }, time.Second, 5*time.Millisecond)
}

func TestAgentRunProjection_SetRunningErrorLogged(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{runningErr: errFake}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, func(context.Context, string) {}))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.running("c1") }, time.Second, 5*time.Millisecond)
}

func TestAgentRunProjection_ResetIdleErrorLogged(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{resetErr: errFake}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, func(context.Context, string) {}))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = runs.Fail(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return fakeChat.reset("c1") }, time.Second, 5*time.Millisecond)
}

func TestAgentRunProjection_SubscribeErrorReturned(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	require.NoError(t, ax.Shutdown(context.Background()))

	err = repositories.RegisterAgentRunProjection(ax, &captureChatRepo{}, nil, func(context.Context, string) {})
	assert.Error(t, err)
}

func TestAgentRunProjection_PendingDoesNothing(t *testing.T) {
	ctx := context.Background()
	ax, runs := newAgentRunAsynx(t)
	fakeChat := &captureChatRepo{}
	var refreshCount int
	var mu sync.Mutex
	refresh := func(context.Context, string) {
		mu.Lock()
		defer mu.Unlock()
		refreshCount++
	}
	require.NoError(t, repositories.RegisterAgentRunProjection(ax, fakeChat, runs, refresh))

	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return refreshCount >= 1
	}, time.Second, 5*time.Millisecond)
	assert.False(t, fakeChat.running("c1"))
	assert.False(t, fakeChat.reset("c1"))
}
