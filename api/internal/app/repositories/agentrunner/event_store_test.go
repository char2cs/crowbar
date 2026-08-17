package agentrunner_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	arcmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// captureBroadcast records every (runnerID, workspaceID, chatID, kind) frame the
// hub projection fans out, so a test can assert the facade wired the hub seam
// through to store.New.
type captureBroadcast struct {
	mu   sync.Mutex
	rows []string
}

func (c *captureBroadcast) push(runnerID, workspaceID, chatID, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, runnerID+":"+workspaceID+":"+chatID+":"+kind)
}

func (c *captureBroadcast) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.rows...)
}

// newRepoWithDeps builds an EventStore over throwaway in-memory event + read-model
// DBs and returns the captured hub broadcast so tests can inspect fan-out.
//
// Synchronisation is on asynx's REAL signals only: mutations return once the
// command is committed, and agentrunner.WaitQuiescentForTest (ax.WaitPublish)
// blocks until every projection handler has run. No sleeps, no polling, no
// timeouts anywhere in this file.
func newRepoWithDeps(
	t *testing.T,
) (context.Context, agentrunner.EventStore, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	cap := &captureBroadcast{}
	repo, err := agentrunner.NewEventSourced(ax, es, db, cap.push)
	require.NoError(t, err)
	return context.Background(), repo, cap
}

func newRepo(
	t *testing.T,
) (context.Context, agentrunner.EventStore) {
	t.Helper()
	ctx, repo, _ := newRepoWithDeps(t)
	return ctx, repo
}

func startRunner(
	t *testing.T,
	ctx context.Context,
	repo agentrunner.EventStore,
	runnerID string,
	chatID string,
	now time.Time,
) domain.AgentRunner {
	t.Helper()
	r, err := repo.Start(ctx, agentrunner.StartInput{
		RunnerID: runnerID, WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "term-" + runnerID, ChatID: chatID, Now: now,
	})
	require.NoError(t, err)
	return r
}

// TestAgentRunner_StartMoveGet_RoundTrip is the core round-trip: a runner is
// started into one chat, MOVED to another (the single atomic write this whole
// aggregate exists for), and the read model then reports the MOVED placement.
func TestAgentRunner_StartMoveGet_RoundTrip(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	started := startRunner(t, ctx, repo, "r1", "chat-a", now)
	assert.Equal(t, "chat-a", started.CurrentChatID)
	assert.Empty(t, started.CurrentSession, "a fresh runner has announced no conversation yet")

	moved, err := repo.Move(ctx, "r1", "chat-b", "sess-2", false, now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "chat-b", moved.CurrentChatID)
	assert.Equal(t, "sess-2", moved.CurrentSession)

	agentrunner.WaitQuiescentForTest(repo)
	got, err := repo.Get(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "chat-b", got.CurrentChatID, "the read model must reflect the moved placement")
	assert.Equal(t, "sess-2", got.CurrentSession)
	assert.Equal(t, "term-r1", got.TerminalSession, "the PTY travels unchanged across a move")

	// The move is visible from the placement read paths too: the runner is live
	// for the chat it ENTERED and absent from the one it LEFT.
	live, err := repo.LiveRunnerForChat(ctx, "chat-b")
	require.NoError(t, err)
	assert.Equal(t, "r1", live.ID)

	_, err = repo.LiveRunnerForChat(ctx, "chat-a")
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat the runner left is dormant again")
}

// TestAgentRunner_Get_MissingBridgesToPackageErrNotFound pins the layering: the
// store keeps its own LOCAL ErrNotFound sentinel (no import cycle back into this
// package), and mapNotFound bridges it so every caller sees the package's own
// EXPORTED sentinel — never the internal one.
func TestAgentRunner_Get_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Get(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestAgentRunner_MissingReadsBridgeToPackageErrNotFound covers the bridge on
// EVERY read path that can miss, not just Get.
func TestAgentRunner_MissingReadsBridgeToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.LiveRunnerForChat(ctx, "no-such-chat")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)

	_, err = repo.LiveRunnerForSession(ctx, "w1", "no-such-session")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)

	_, err = repo.ChatForSession(ctx, "w1", "no-such-session")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)

	_, err = repo.LastConversation(ctx, "no-such-chat")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestAgentRunner_BindSession_OpensTheConversation: binding announces the
// runner's first conversation, which lands in BOTH the live placement and the
// append-only history.
func TestAgentRunner_BindSession_OpensTheConversation(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)

	bound, err := repo.BindSession(ctx, "r1", "sess-1", false, now.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "sess-1", bound.CurrentSession)
	assert.Equal(t, now.Add(time.Second), bound.CurrentSessionSince,
		"the conversation's opening time is when it was BOUND, never when the runner spawned")

	agentrunner.WaitQuiescentForTest(repo)

	live, err := repo.LiveRunnerForSession(ctx, "w1", "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "r1", live.ID)

	chatID, err := repo.ChatForSession(ctx, "w1", "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "chat-a", chatID)

	conv, err := repo.LastConversation(ctx, "chat-a")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", conv.SessionID)
	assert.Equal(t, "claude", conv.ProviderID)
}

// TestAgentRunner_BindSessionAndMove_RejectZeroNow pins the STRUCTURAL guard: a
// conversation with no opening time would stamp FirstSeenAt zero and silently
// drop history ordering back onto insertion order, so the commands refuse it.
func TestAgentRunner_BindSessionAndMove_RejectZeroNow(t *testing.T) {
	ctx, repo := newRepo(t)
	startRunner(t, ctx, repo, "r1", "chat-a", time.Unix(1000, 0).UTC())

	_, err := repo.BindSession(ctx, "r1", "sess-1", false, time.Time{})
	require.ErrorIs(t, err, asynxModels.ErrValidation)

	_, err = repo.Move(ctx, "r1", "chat-b", "sess-2", false, time.Time{})
	require.ErrorIs(t, err, asynxModels.ErrValidation)
}

// TestAgentRunner_Exit_DropsTheLiveRow_KeepsHistory: the PTY died, so the live
// row is GONE — its absence IS the dormancy, there is no status to flip. The
// append-only conversation history survives, so the chat stays resumable.
func TestAgentRunner_Exit_DropsTheLiveRow_KeepsHistory(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)
	_, err := repo.BindSession(ctx, "r1", "sess-1", false, now.Add(time.Second))
	require.NoError(t, err)

	exited, err := repo.Exit(ctx, "r1", now.Add(2*time.Second))
	require.NoError(t, err)
	require.NotNil(t, exited.ExitedAt, "the tombstone is audit-only, never a liveness flag")

	agentrunner.WaitQuiescentForTest(repo)

	_, err = repo.Get(ctx, "r1")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "an exited runner has no live row")
	_, err = repo.LiveRunnerForChat(ctx, "chat-a")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat is dormant again")

	// History survives the process.
	conv, err := repo.LastConversation(ctx, "chat-a")
	require.NoError(t, err)
	assert.Equal(t, "sess-1", conv.SessionID)
}

// TestAgentRunner_AllLive_ListsRunningRunnersOnly backs boot reconciliation: it
// returns what the read model believes is running, which is the input to the
// PTY (the sole authority) being asked whether each one really is.
func TestAgentRunner_AllLive_ListsRunningRunnersOnly(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)
	startRunner(t, ctx, repo, "r2", "chat-b", now)
	agentrunner.WaitQuiescentForTest(repo)

	live, err := repo.AllLive(ctx)
	require.NoError(t, err)
	assert.Len(t, live, 2)

	_, err = repo.Exit(ctx, "r1", now.Add(time.Second))
	require.NoError(t, err)
	agentrunner.WaitQuiescentForTest(repo)

	live, err = repo.AllLive(ctx)
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, "r2", live[0].ID)
}

// TestAgentRunner_ForgetChat_DropsHistoryNotTheRunner: the chat-delete cascade
// removes the chat's conversation history — the one case where history has
// nothing left to describe — but never hand-deletes the live runner row. That
// row belongs to the PTY's lifecycle; the cascade kills the process and lets the
// row follow (Task 5).
func TestAgentRunner_ForgetChat_DropsHistoryNotTheRunner(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)
	_, err := repo.BindSession(ctx, "r1", "sess-1", false, now.Add(time.Second))
	require.NoError(t, err)
	agentrunner.WaitQuiescentForTest(repo)

	require.NoError(t, repo.ForgetChat(ctx, "chat-a"))

	_, err = repo.LastConversation(ctx, "chat-a")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat's history is gone")
	_, err = repo.ChatForSession(ctx, "w1", "sess-1")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)

	got, err := repo.Get(ctx, "r1")
	require.NoError(t, err, "ForgetChat must NOT hand-delete the live runner row")
	assert.Equal(t, "chat-a", got.CurrentChatID)
}

// TestAgentRunner_HubBroadcastsRunnerChatAndKind proves NewEventSourced wired the
// hub seam through to store.New: every lifecycle event fans out a frame carrying
// the runner, its workspace, the chat it is pointed at AS OF that event, and the
// kind derived from the command's event name.
func TestAgentRunner_HubBroadcastsRunnerChatAndKind(t *testing.T) {
	ctx, repo, cap := newRepoWithDeps(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)
	_, err := repo.Move(ctx, "r1", "chat-b", "sess-2", false, now.Add(time.Second))
	require.NoError(t, err)
	_, err = repo.Exit(ctx, "r1", now.Add(2*time.Second))
	require.NoError(t, err)

	agentrunner.WaitQuiescentForTest(repo)
	assert.Equal(t, []string{
		"r1:w1:chat-a:started",
		"r1:w1:chat-b:moved",
		"r1:w1:chat-b:exited",
	}, cap.all(), "a moved frame must name the chat the runner ENTERED")
}

// concurrentMoveTrials is the number of independent fresh-runner trials
// TestAgentRunner_ConcurrentMove_OCCRetryConverges runs, giving the race repeated
// chances to surface nondeterminism.
const concurrentMoveTrials = 10

// TestAgentRunner_ConcurrentMove_OCCRetryConverges exercises the OCC retry
// contract under a REAL race. Two Moves fire concurrently at the SAME runner
// aggregate. asynx v0.7.0 enforces write-path optimistic concurrency — the loser
// collides on (id,version) with ErrPipelineFailed — so sendWithOCC must RETRY it
// against the now-committed state rather than surfacing the collision or
// clobbering the winner.
//
// Move carries no state guard (a runner may move any number of times), so BOTH
// sends must ultimately commit: the retry converges. The aggregate is left at
// exactly one of the two placements — never a torn half-write — and the read
// model holds exactly one live row for it.
//
// No sleeps: the goroutines are joined by errgroup.Wait and the read is preceded
// by WaitQuiescentForTest (asynx WaitPublish). Convergence is guaranteed by
// (id,version) uniqueness, not by timing.
func TestAgentRunner_ConcurrentMove_OCCRetryConverges(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	for trial := range concurrentMoveTrials {
		runnerID := fmt.Sprintf("race-runner-%d", trial)
		chatA := fmt.Sprintf("race-a-%d", trial)
		chatB := fmt.Sprintf("race-b-%d", trial)
		startRunner(t, ctx, repo, runnerID, "race-home", now)

		results := make([]error, 2)
		var g errgroup.Group
		g.Go(func() error {
			_, results[0] = repo.Move(ctx, runnerID, chatA, "sess-a", false, now.Add(time.Second))
			return nil
		})
		g.Go(func() error {
			_, results[1] = repo.Move(ctx, runnerID, chatB, "sess-b", false, now.Add(time.Second))
			return nil
		})
		require.NoError(t, g.Wait())

		for i, e := range results {
			require.NoErrorf(t, e, "trial %d: move %d must converge via OCC retry, never surface the collision", trial, i)
		}

		agentrunner.WaitQuiescentForTest(repo)
		got, err := repo.Get(ctx, runnerID)
		require.NoError(t, err)
		require.Containsf(t, []string{chatA, chatB}, got.CurrentChatID,
			"trial %d: the runner must land on exactly one of the two chats — never a torn write", trial)

		// One runner, one live row: the placement is single-valued whichever move won.
		live, err := repo.LiveRunnerForChat(ctx, got.CurrentChatID)
		require.NoError(t, err)
		require.Equal(t, runnerID, live.ID)
	}
}

// TestAgentRunner_OccSendErrorDisposition pins the terminal error-disposition
// contract against a fake send: ErrPipelineFailed is retried exactly
// MaxOCCAttempts then surfaced (→ 409); ErrValidation is never retried (→ 422);
// ErrQueueFull is never retried and is translated to apperr.ErrUnavailable
// (→ 503). All classified via errors.Is, never string compare.
func TestAgentRunner_OccSendErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := arcmds.Exit{RunnerID: "r1", Now: time.Unix(1, 0)}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentRunner]) (asynxModels.Event[domain.AgentRunner], error) {
			calls++
			return asynxModels.Event[domain.AgentRunner]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := agentrunner.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, agentrunner.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentRunner]) (asynxModels.Event[domain.AgentRunner], error) {
			calls++
			return asynxModels.Event[domain.AgentRunner]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := agentrunner.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.AgentRunner]) (asynxModels.Event[domain.AgentRunner], error) {
			calls++
			return asynxModels.Event[domain.AgentRunner]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := agentrunner.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.AgentRunner]) (asynxModels.Event[domain.AgentRunner], error) {
			return asynxModels.Event[domain.AgentRunner]{Aggregate: domain.AgentRunner{ID: c.AggregateID()}}, nil
		}
		evt, err := agentrunner.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "r1", evt.Aggregate.ID)
	})
}

// TestAgentRunner_Start_ErrorOnDuplicate: a runner id is minted once per spawn,
// so re-Starting one is a caller bug, surfaced as validation (never retried).
func TestAgentRunner_Start_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	startRunner(t, ctx, repo, "r1", "chat-a", now)

	_, err := repo.Start(ctx, agentrunner.StartInput{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "term-r1", ChatID: "chat-a", Now: now,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentRunner_NewEventSourced_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = agentrunner.NewEventSourced(ax, es, db, func(string, string, string, string) {})
	require.Error(t, err)
}

// TestAgentRunner_NewEventSourced_ErrorOnNilBroadcast: the hub projection calls
// broadcast on every event, so a nil one would panic inside a projection
// goroutine far from whoever built the repo. store.New refuses it and the facade
// surfaces that refusal.
func TestAgentRunner_NewEventSourced_ErrorOnNilBroadcast(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = agentrunner.NewEventSourced(ax, es, db, nil)
	require.Error(t, err)
}
