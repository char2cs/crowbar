package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errFake = errors.New("fake error")

type fakeAgentRunRepo struct {
	agentrun.AgentRun
	getResult domain.AgentRun
	failErr   error
}

func (f *fakeAgentRunRepo) Get(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return f.getResult, nil
}

func (f *fakeAgentRunRepo) Fail(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, f.failErr
}

type fakeChatRepo struct {
	chat.Chat
	getResult    domain.Chat
	resetIdleErr error
}

func (f *fakeChatRepo) Get(
	_ context.Context,
	_ string,
) (domain.Chat, error) {
	return f.getResult, nil
}

func (f *fakeChatRepo) ResetIdle(
	_ context.Context,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{}, f.resetIdleErr
}

func newAgentRunRepo(t *testing.T) agentrun.AgentRun {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return agentrun.New(ax)
}

func newChatRepo(t *testing.T) chat.Chat {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return chat.New(ax)
}

func TestRecoverAgentRuns_RunningBecomesError(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)

	repositories.RecoverAgentRuns(ctx, []string{"a1"}, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, got.Status)
}

func TestRecoverAgentRuns_NonRunningUntouched(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)

	repositories.RecoverAgentRuns(ctx, []string{"a1"}, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusPending, got.Status)
}

func TestReconcileChats_AgentRunningWithNoLiveRun_ResetToIdle(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestReconcileChats_LiveRunKeepsAgentRunning(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return true }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusAgentRunning, got.Status)
}

func TestReconcileChats_SecondPassIsIdempotent(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)

	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)
	repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, chats)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestRecoverAgentRuns_GetErrorLogsAndContinues(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(ctx, []string{"does-not-exist"}, runs)
	})
}

func TestRecoverAgentRuns_FailErrorLogsAndContinues(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAgentRunRepo{
		getResult: domain.AgentRun{Status: domain.AgentRunStatusRunning},
		failErr:   errFake,
	}
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(ctx, []string{"a1"}, fake)
	})
}

func TestReconcileChats_GetErrorLogsAndContinues(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(ctx, []string{"does-not-exist"}, func(string) bool { return false }, chats)
	})
}

func TestReconcileChats_ResetIdleErrorLogsAndContinues(t *testing.T) {
	ctx := context.Background()
	fake := &fakeChatRepo{
		getResult:    domain.Chat{Status: domain.ChatStatusAgentRunning},
		resetIdleErr: errFake,
	}
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(ctx, []string{"c1"}, func(string) bool { return false }, fake)
	})
}
