package agentchat_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// captureBroadcast records every (chatID, kind) frame the hub projection fans
// out, mirroring reviewthread's test capture.
type captureBroadcast struct {
	mu   sync.Mutex
	rows []string
}

func (c *captureBroadcast) push(chatID string, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, chatID+":"+kind)
}

func (c *captureBroadcast) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}

// newRepoWithDeps builds an EventStore repo over throwaway in-memory event +
// read-model DBs and returns the read-model DB and the captured hub broadcast
// so tests can inspect persistence + fan-out.
func newRepoWithDeps(
	t *testing.T,
) (context.Context, agentchat.EventStore, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	cap := &captureBroadcast{}
	repo, err := agentchat.NewEventSourced(ax, es, db, cap.push)
	require.NoError(t, err)
	return context.Background(), repo, db, cap
}

func newRepo(
	t *testing.T,
) (context.Context, agentchat.EventStore) {
	t.Helper()
	ctx, repo, _, _ := newRepoWithDeps(t)
	return ctx, repo
}

func createChat(
	t *testing.T,
	ctx context.Context,
	repo agentchat.EventStore,
	id string,
	wsID string,
	now time.Time,
) domain.AgentChat {
	t.Helper()
	chat, err := repo.Create(ctx, agentchat.CreateInput{
		ID: id, WorkspaceID: wsID, SegmentID: id + "-s1", CrowbarSegmentID: id + "-cs1",
		ProviderID: "claude", TerminalSession: "term-1", Now: now,
	})
	require.NoError(t, err)
	return chat
}

func TestAgentChat_CreateAndGetChat(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	created := createChat(t, ctx, repo, "c1", "w1", now)
	require.Len(t, created.Segments, 1)
	assert.Equal(t, "active", created.Segments[0].Status)

	agentchat.WaitQuiescentForTest(repo)
	got, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.WorkspaceID)
	assert.Equal(t, domain.AgentChatStatusActive, got.Status)
}

func TestAgentChat_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	_, err := repo.Create(ctx, agentchat.CreateInput{
		ID: "c1", WorkspaceID: "w1", SegmentID: "s2", CrowbarSegmentID: "cs2",
		ProviderID: "claude", TerminalSession: "term-2", Now: now,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_GetChat_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.GetChat(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

func TestAgentChat_GetByProviderSession_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())
	agentchat.WaitQuiescentForTest(repo)

	_, err := repo.GetByProviderSession(ctx, "no-such-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

func TestAgentChat_SegmentLifecycle_EndOpenBind(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	_, err := repo.EndSegment(ctx, "c1", "c1-s1", now.Add(time.Second))
	require.NoError(t, err)

	opened, err := repo.OpenSegment(ctx, agentchat.OpenSegmentInput{
		ChatID: "c1", SegmentID: "s2", CrowbarSegmentID: "cs2",
		ProviderID: "codex", TerminalSession: "term-2", Now: now.Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.Len(t, opened.Segments, 2)
	assert.Equal(t, "ended", opened.Segments[0].Status)
	assert.Equal(t, "active", opened.Segments[1].Status)

	bound, err := repo.BindSession(ctx, "c1", "cs2", "provider-sess-1")
	require.NoError(t, err)
	require.Len(t, bound.Segments, 2)
	assert.Equal(t, "provider-sess-1", bound.Segments[1].ProviderSessionID)

	agentchat.WaitQuiescentForTest(repo)
	byID, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "provider-sess-1", byID.Segments[1].ProviderSessionID)

	bySession, err := repo.GetByProviderSession(ctx, "provider-sess-1")
	require.NoError(t, err)
	assert.Equal(t, "c1", bySession.ID)
}

func TestAgentChat_StartStopTurn(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	started, err := repo.StartTurn(ctx, "c1", now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, started.Working)
	require.NotNil(t, started.CurrentTurnStarted)

	stopped, err := repo.StopTurn(ctx, "c1", now.Add(2*time.Second))
	require.NoError(t, err)
	assert.False(t, stopped.Working)
	assert.Nil(t, stopped.CurrentTurnStarted)
}

func TestAgentChat_SetTitle_UserSourceLocksAgainstAgent(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	derived, err := repo.SetTitle(ctx, "c1", "Derived title", "derived")
	require.NoError(t, err)
	assert.Equal(t, "Derived title", derived.Title)
	assert.False(t, derived.TitleLocked)

	userTitled, err := repo.SetTitle(ctx, "c1", "User title", "user")
	require.NoError(t, err)
	assert.Equal(t, "User title", userTitled.Title)
	assert.True(t, userTitled.TitleLocked)

	_, err = repo.SetTitle(ctx, "c1", "Agent override", "agent")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_Delete_TombstoneReachableByIDExcludedFromList(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)
	createChat(t, ctx, repo, "c2", "w1", now)

	require.NoError(t, repo.Delete(ctx, "c2"))
	agentchat.WaitQuiescentForTest(repo)

	list, err := repo.ListChats(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)

	deleted, err := repo.GetChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentChatStatusDeleted, deleted.Status)
}

func TestAgentChat_Delete_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	err := repo.Delete(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)
	createChat(t, ctx, repo, "c2", "w2", now)
	agentchat.WaitQuiescentForTest(repo)

	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)

	none, err := repo.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestAgentChat_HubBroadcastsChatIDAndKind(t *testing.T) {
	ctx, repo, _, cap := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())

	agentchat.WaitQuiescentForTest(repo)
	assert.GreaterOrEqual(t, cap.count(), 1, "hub projection must broadcast a lifecycle frame")
}

// concurrentOpenSegmentTrials is the number of independent fresh-chat trials
// TestAgentChat_ConcurrentOpenSegment_OneWins runs. asynx v0.7.0 enforces real
// write-path optimistic concurrency: eventstore.Write loads the aggregate state
// and its version together and appends at expectedVersion+1, so two Sends to the
// same aggregate collide on (id,version) uniqueness — the loser gets
// ErrPipelineFailed and retries (re-loading the now-committed state), where
// OpenSegment.Validate rejects it. So exactly one of two concurrent OpenSegments
// commits, deterministically, at the default 8 workers/shard — no writeMu and no
// WorkersPerShard tuning needed. Several trials give the race repeated chances to
// surface any nondeterminism. (v0.6.2 had an upstream OCC gap here — the append
// version was recomputed independent of the validated load, letting both commit;
// fixed in v0.7.0, commit 75ffc45.)
const concurrentOpenSegmentTrials = 10

// TestAgentChat_ConcurrentOpenSegment_OneWins exercises the OCC retry
// contract (no per-aggregate writeMu) under a REAL race: with the initial
// segment ended, two OpenSegment sends targeting the SAME chat fire
// concurrently via errgroup, repeated across several independent freshly
// created chats. Run with -race. No sleeps: each trial's goroutines are
// joined by errgroup.Wait, and each trial's read is preceded by
// WaitQuiescentForTest (asynx WaitPublish), never a timing guess.
//
// asynx v0.7.0 enforces real write-path OCC (Write loads state+version together
// and appends at expectedVersion+1), so every trial hard-asserts the fully
// correct outcome deterministically: exactly one send commits, the loser is
// rejected by OpenSegment.Validate (wrapping asynxModels.ErrValidation, never an
// unresolved ErrPipelineFailed), and the read model ends with exactly one active
// segment. No sleeps; convergence is guaranteed by (id,version) uniqueness, not
// timing.
func TestAgentChat_ConcurrentOpenSegment_OneWins(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)
	now := time.Unix(1000, 0).UTC()

	for trial := range concurrentOpenSegmentTrials {
		chatID := fmt.Sprintf("race-chat-%d", trial)
		createChat(t, ctx, repo, chatID, "w1", now)
		_, err := repo.EndSegment(ctx, chatID, chatID+"-s1", now.Add(time.Second))
		require.NoError(t, err)

		results := make([]error, 2)
		var g errgroup.Group
		g.Go(func() error {
			_, results[0] = repo.OpenSegment(ctx, agentchat.OpenSegmentInput{
				ChatID: chatID, SegmentID: "sA", CrowbarSegmentID: "csA",
				ProviderID: "claude", TerminalSession: "term-A", Now: now.Add(2 * time.Second),
			})
			return nil
		})
		g.Go(func() error {
			_, results[1] = repo.OpenSegment(ctx, agentchat.OpenSegmentInput{
				ChatID: chatID, SegmentID: "sB", CrowbarSegmentID: "csB",
				ProviderID: "codex", TerminalSession: "term-B", Now: now.Add(2 * time.Second),
			})
			return nil
		})
		require.NoError(t, g.Wait())

		succeeded := 0
		for _, e := range results {
			if e == nil {
				succeeded++
				continue
			}
			require.ErrorIsf(t, e, asynxModels.ErrValidation,
				"trial %d: a losing OpenSegment must be rejected by the active-segment guard, not left as an unresolved pipeline failure", trial)
		}
		require.Equalf(t, 1, succeeded, "trial %d: exactly one racing OpenSegment must commit; the other must be rejected", trial)

		agentchat.WaitQuiescentForTest(repo)
		got, err := repo.GetChat(ctx, chatID)
		require.NoError(t, err)
		active := 0
		for _, seg := range got.Segments {
			if seg.Status == "active" {
				active++
			}
		}
		require.Equalf(t, 1, active, "trial %d: the read model must show exactly one active segment (≤1-active invariant holds under the race)", trial)
	}
}

// TestAgentChat_OccSendErrorDisposition pins the terminal error-disposition
// contract against a fake send: ErrPipelineFailed is retried exactly
// MaxOCCAttempts then surfaced (→ 409); ErrValidation is never retried
// (→ 422); ErrQueueFull is never retried and is translated to
// apperr.ErrUnavailable (→ 503). All classified via errors.Is.
func TestAgentChat_OccSendErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := accmds.StartTurn{ChatID: "c1", Now: time.Unix(1, 0)}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentChat]) (asynxModels.Event[domain.AgentChat], error) {
			calls++
			return asynxModels.Event[domain.AgentChat]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := agentchat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, agentchat.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentChat]) (asynxModels.Event[domain.AgentChat], error) {
			calls++
			return asynxModels.Event[domain.AgentChat]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := agentchat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentChat]) (asynxModels.Event[domain.AgentChat], error) {
			calls++
			return asynxModels.Event[domain.AgentChat]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := agentchat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.AgentChat]) (asynxModels.Event[domain.AgentChat], error) {
			return asynxModels.Event[domain.AgentChat]{Aggregate: domain.AgentChat{ID: c.AggregateID()}}, nil
		}
		evt, err := agentchat.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "c1", evt.Aggregate.ID)
	})
}

func TestAgentChat_NewEventSourced_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
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

	_, err = agentchat.NewEventSourced(ax, es, db, func(string, string) {})
	require.Error(t, err)
}
