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
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errFake = errors.New("fake error")

type listErrAgentRunRepo struct {
	agentrun.AgentRun
}

func (listErrAgentRunRepo) ListRunning(
	_ context.Context,
) ([]domain.AgentRun, error) {
	return nil, errFake
}

type listErrChatRepo struct {
	chat.Chat
}

func (listErrChatRepo) List(
	_ context.Context,
) ([]domain.Chat, error) {
	return nil, errFake
}

type stubRunRepo struct {
	agentrun.AgentRun
	running   []domain.AgentRun
	getErr    error
	failErr   error
	getStatus domain.AgentRunStatus
}

func (s stubRunRepo) ListRunning(
	_ context.Context,
) ([]domain.AgentRun, error) {
	return s.running, nil
}

func (s stubRunRepo) Get(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	if s.getErr != nil {
		return domain.AgentRun{}, s.getErr
	}
	status := s.getStatus
	if status == "" {
		status = domain.AgentRunStatusRunning
	}
	return domain.AgentRun{ID: "a1", Status: status}, nil
}

func (s stubRunRepo) Fail(
	_ context.Context,
	_ string,
) (domain.AgentRun, error) {
	return domain.AgentRun{}, s.failErr
}

type stubChatRepo struct {
	chat.Chat
	list      []domain.Chat
	getErr    error
	getStatus domain.ChatStatus
}

func (s stubChatRepo) List(
	_ context.Context,
) ([]domain.Chat, error) {
	return s.list, nil
}

func (s stubChatRepo) Get(
	_ context.Context,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{Status: s.getStatus}, s.getErr
}

func (s stubChatRepo) ResetIdle(
	_ context.Context,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{}, errFake
}

type recordResetChatRepo struct {
	chat.Chat
	list     []domain.Chat
	resetIDs []string
}

func (s *recordResetChatRepo) List(
	_ context.Context,
) ([]domain.Chat, error) {
	return s.list, nil
}

func (s *recordResetChatRepo) Get(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	for _, c := range s.list {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Chat{}, errFake
}

func (s *recordResetChatRepo) ResetIdle(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	s.resetIDs = append(s.resetIDs, id)
	return domain.Chat{}, nil
}

func TestReconcileChats_SoftDeletedChatIsSkipped(t *testing.T) {
	deletedAt := time.Unix(2, 0)
	chats := &recordResetChatRepo{
		list: []domain.Chat{
			{ID: "c1", Status: domain.ChatStatusAgentRunning, DeletedAt: &deletedAt},
		},
	}
	repositories.ReconcileChats(context.Background(), chats, newAgentRunRepo(t))
	assert.Empty(t, chats.resetIDs, "soft-deleted chat must not be reset")
}

func TestReconcileChats_LiveChatStillReconciledAlongsideTombstone(t *testing.T) {
	deletedAt := time.Unix(2, 0)
	chats := &recordResetChatRepo{
		list: []domain.Chat{
			{ID: "dead", Status: domain.ChatStatusAgentRunning, DeletedAt: &deletedAt},
			{ID: "live", Status: domain.ChatStatusAgentRunning},
		},
	}
	repositories.ReconcileChats(context.Background(), chats, newAgentRunRepo(t))
	assert.Equal(t, []string{"live"}, chats.resetIDs)
}

func TestReconcileChats_ResetIdleErrorLogsAndContinues(t *testing.T) {
	chats := stubChatRepo{
		list:      []domain.Chat{{ID: "c1"}},
		getStatus: domain.ChatStatusAgentRunning,
	}
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(context.Background(), chats, newAgentRunRepo(t))
	})
}

func TestRecoverAgentRuns_GetErrorLogsAndContinues(t *testing.T) {
	runs := stubRunRepo{running: []domain.AgentRun{{ID: "a1"}}, getErr: errFake}
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(context.Background(), runs)
	})
}

func TestRecoverAgentRuns_FailErrorLogsAndContinues(t *testing.T) {
	runs := stubRunRepo{running: []domain.AgentRun{{ID: "a1"}}, failErr: errFake}
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(context.Background(), runs)
	})
}

func TestReconcileChats_GetErrorLogsAndContinues(t *testing.T) {
	chats := stubChatRepo{list: []domain.Chat{{ID: "c1"}}, getErr: errFake}
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(context.Background(), chats, newAgentRunRepo(t))
	})
}

func TestRecoverAgentRuns_NonRunningEnumeratedIsSkipped(t *testing.T) {
	runs := stubRunRepo{
		running:   []domain.AgentRun{{ID: "a1"}},
		getStatus: domain.AgentRunStatusDone,
	}
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(context.Background(), runs)
	})
}

func TestReconcileChats_NonAgentRunningEnumeratedIsSkipped(t *testing.T) {
	chats := stubChatRepo{
		list:      []domain.Chat{{ID: "c1"}},
		getStatus: domain.ChatStatusIdle,
	}
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(context.Background(), chats, newAgentRunRepo(t))
	})
}

func newAgentRunRepo(
	t *testing.T,
) agentrun.AgentRun {
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
	return runs
}

func newChatRepo(
	t *testing.T,
) chat.Chat {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	chats, err := chat.New(ax, db, func(domain.Chat) {})
	require.NoError(t, err)
	return chats
}

func eventuallyRunningEmpty(
	t *testing.T,
	runs agentrun.AgentRun,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		running, err := runs.ListRunning(context.Background())
		return err == nil && len(running) == 0
	}, time.Second, 5*time.Millisecond)
}

func TestRecoverAgentRuns_RunningBecomesError(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := runs.ListRunning(ctx)
		return listErr == nil && len(running) == 1
	}, time.Second, 5*time.Millisecond)

	repositories.RecoverAgentRuns(ctx, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, got.Status)
	eventuallyRunningEmpty(t, runs)
}

func TestRecoverAgentRuns_NoRunningIsNoOp(t *testing.T) {
	ctx := context.Background()
	runs := newAgentRunRepo(t)
	_, err := runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)

	repositories.RecoverAgentRuns(ctx, runs)

	got, err := runs.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusPending, got.Status)
}

func TestRecoverAgentRuns_ListErrorLogsAndReturns(t *testing.T) {
	assert.NotPanics(t, func() {
		repositories.RecoverAgentRuns(context.Background(), listErrAgentRunRepo{})
	})
}

func TestReconcileChats_AgentRunningWithNoLiveRun_ResetToIdle(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	runs := newAgentRunRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", "t", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		got, getErr := chats.Get(ctx, "c1")
		return getErr == nil && got.Status == domain.ChatStatusAgentRunning
	}, time.Second, 5*time.Millisecond)

	repositories.ReconcileChats(ctx, chats, runs)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestReconcileChats_LiveRunKeepsAgentRunning(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	runs := newAgentRunRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", "t", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	_, err = runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := runs.ListRunning(ctx)
		return listErr == nil && len(running) == 1
	}, time.Second, 5*time.Millisecond)

	repositories.ReconcileChats(ctx, chats, runs)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusAgentRunning, got.Status)
}

func TestReconcileChats_SecondPassIsIdempotent(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	runs := newAgentRunRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", "t", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		got, getErr := chats.Get(ctx, "c1")
		return getErr == nil && got.Status == domain.ChatStatusAgentRunning
	}, time.Second, 5*time.Millisecond)

	repositories.ReconcileChats(ctx, chats, runs)
	repositories.ReconcileChats(ctx, chats, runs)

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}

func TestReconcileChats_ListChatsErrorLogsAndReturns(t *testing.T) {
	assert.NotPanics(t, func() {
		repositories.ReconcileChats(context.Background(), listErrChatRepo{}, newAgentRunRepo(t))
	})
}

func TestReconcileChats_LiveSetListErrorStillReconciles(t *testing.T) {
	ctx := context.Background()
	chats := newChatRepo(t)
	_, err := chats.Create(ctx, "c1", "w1", "t", time.Unix(1, 0))
	require.NoError(t, err)
	_, err = chats.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		got, getErr := chats.Get(ctx, "c1")
		return getErr == nil && got.Status == domain.ChatStatusAgentRunning
	}, time.Second, 5*time.Millisecond)

	repositories.ReconcileChats(ctx, chats, listErrAgentRunRepo{})

	got, err := chats.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, got.Status)
}
