package chat_test

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
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// captureBroadcast records every (chatID, workspaceID, kind) frame the hub
// projection fans out, mirroring reviewthread's test capture.
type captureBroadcast struct {
	mu   sync.Mutex
	rows []string
}

// watch adapts the repository's announcement seam onto the frame this fixture's
// assertions already use, so the change is proven against the existing suite.
func (c *captureBroadcast) watch(e chat.ChatEvent) {
	c.push(e.ChatID, e.WorkspaceID, e.Kind, e.Working && !e.Forgotten)
}

func (c *captureBroadcast) push(chatID, workspaceID, kind string, _ bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, chatID+":"+workspaceID+":"+kind)
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
) (context.Context, chat.EventStore, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	cap := &captureBroadcast{}
	repo, err := chat.NewEventSourced(ax, es, db, cap.watch)
	require.NoError(t, err)
	return context.Background(), repo, db, cap
}

func newRepo(
	t *testing.T,
) (context.Context, chat.EventStore) {
	t.Helper()
	ctx, repo, _, _ := newRepoWithDeps(t)
	return ctx, repo
}

func createChat(
	t *testing.T,
	ctx context.Context,
	repo chat.EventStore,
	id string,
	wsID string,
	now time.Time,
) domain.Chat {
	t.Helper()
	chat, err := repo.Create(ctx, chat.CreateInput{ID: id, WorkspaceID: wsID, Type: domain.ChatTypeChat, Now: now})
	require.NoError(t, err)
	return chat
}

func TestAgentChat_CreateAndGetChat(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	created := createChat(t, ctx, repo, "c1", "w1", now)
	assert.Equal(t, "c1", created.ID)
	assert.Equal(t, now, created.CreatedAt)

	// Create is the one command on the SendWait path, so the read model already
	// reflects it: the hook that lands microseconds after a /clear mints a chat must
	// not see "no such chat" and drop the turn.
	got, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.WorkspaceID)
}

func TestAgentChat_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: now})
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_GetChat_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.GetChat(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

// The segment lifecycle (EndSegment / OpenSegment / BindSession) and the
// GetByProviderSession lookup that scanned segments are GONE, along with AgentSegment
// itself. A chat holds no process state: which CLI is on it, and which conversations it
// has hosted, are projections of RUNNER events (agentrunner). Their coverage lives
// there and in the agent usecase's move/eviction tests.

func TestAgentChat_StartStopTurn(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	started, err := repo.StartTurn(ctx, "c1", now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, started.Working)
	require.NotNil(t, started.CurrentTurnStarted)

	stopped, err := repo.StopTurn(ctx, "c1", now.Add(2*time.Second), 0)
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

// TestAgentChat_Forget_ErasesAggregate mirrors reviewthread's
// TestReviewThread_DeleteThread_Forgets: Forget hard-deletes via ax.Forget —
// the synchronous OnForget drops the read-model row AND the underlying event
// log is erased, so a subsequent GetChat cannot self-heal it back via lazy
// Replay and genuinely reports chat.ErrNotFound. This is the primitive
// the workspace-delete cascade (repositories.Container.forgetAgentChats) uses.
func TestAgentChat_Forget_ErasesAggregate(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())
	chat.WaitQuiescentForTest(repo)

	require.NoError(t, repo.Forget(ctx, "c1"))
	chat.WaitQuiescentForTest(repo)

	// The synchronous OnForget row-delete leaves the read model empty.
	var n int64
	require.NoError(t, db.WithContext(ctx).Table("agent_chats_read").Count(&n).Error)
	assert.Zero(t, n, "Forget's OnForget must drop the read-model row")

	// And the event log is erased, so GetChat's miss-triggered Replay-rebuild
	// cannot resurrect it: a genuine not-found.
	_, err := repo.GetChat(ctx, "c1")
	require.Error(t, err)
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

func TestAgentChat_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)
	createChat(t, ctx, repo, "c2", "w2", now)
	chat.WaitQuiescentForTest(repo)

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

	chat.WaitQuiescentForTest(repo)
	assert.GreaterOrEqual(t, cap.count(), 1, "hub projection must broadcast a lifecycle frame")
}

// concurrentCreateTrials is the number of independent trials
// TestAgentChat_ConcurrentCreate_OneWins runs. asynx v0.7.0 enforces real write-path
// optimistic concurrency: eventstore.Write loads the aggregate state and its version
// together and appends at expectedVersion+1, so two Sends to the same aggregate collide
// on (id,version) uniqueness — the loser gets ErrPipelineFailed and retries (re-loading
// the now-committed state), where Validate rejects it. Several trials give the race
// repeated chances to surface any nondeterminism.
const concurrentCreateTrials = 10

// TestAgentChat_ConcurrentCreate_OneWins exercises the OCC retry contract (no
// per-aggregate writeMu) under a REAL race, on the one command that can still collide
// on a single chat aggregate: two Creates of the SAME id, fired concurrently via
// errgroup. Exactly one commits; the loser is rejected by Validate ("exists", wrapping
// asynxModels.ErrValidation) rather than left as an unresolved ErrPipelineFailed.
//
// (Its predecessor raced two OpenSegments against the chat's ≤1-active-segment
// invariant. That invariant — and the segment itself — is gone: a chat holds no process
// state to conflict over, which is exactly why the torn switch that bricked a chat
// cannot happen. The OCC contract it proved still matters, so the race is retargeted at
// the command that still has one.)
//
// Run with -race. No sleeps: the goroutines are joined by errgroup.Wait, and the read is
// preceded by WaitQuiescentForTest (asynx WaitPublish), never a timing guess.
func TestAgentChat_ConcurrentCreate_OneWins(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)
	now := time.Unix(1000, 0).UTC()

	for trial := range concurrentCreateTrials {
		chatID := fmt.Sprintf("race-chat-%d", trial)

		results := make([]error, 2)
		var g errgroup.Group
		for i := range results {
			g.Go(func() error {
				_, results[i] = repo.Create(ctx, chat.CreateInput{
					ID: chatID, WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: now,
				})
				return nil
			})
		}
		require.NoError(t, g.Wait())

		succeeded := 0
		for _, e := range results {
			if e == nil {
				succeeded++
				continue
			}
			require.ErrorIsf(t, e, asynxModels.ErrValidation,
				"trial %d: a losing Create must be rejected by Validate, not left as an unresolved pipeline failure", trial)
		}
		require.Equalf(t, 1, succeeded, "trial %d: exactly one racing Create must commit; the other must be rejected", trial)

		chat.WaitQuiescentForTest(repo)
		got, err := repo.GetChat(ctx, chatID)
		require.NoError(t, err)
		require.Equalf(t, "w1", got.WorkspaceID, "trial %d: the read model must hold exactly one chat for the id", trial)
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
		send := func(context.Context, asynxModels.Command[domain.Chat]) (asynxModels.Event[domain.Chat], error) {
			calls++
			return asynxModels.Event[domain.Chat]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := chat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, chat.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Chat]) (asynxModels.Event[domain.Chat], error) {
			calls++
			return asynxModels.Event[domain.Chat]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := chat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Chat]) (asynxModels.Event[domain.Chat], error) {
			calls++
			return asynxModels.Event[domain.Chat]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := chat.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.Chat]) (asynxModels.Event[domain.Chat], error) {
			return asynxModels.Event[domain.Chat]{Aggregate: domain.Chat{ID: c.AggregateID()}}, nil
		}
		evt, err := chat.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "c1", evt.Aggregate.ID)
	})
}

func TestAgentChat_NewEventSourced_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
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

	_, err = chat.NewEventSourced(ax, es, db, func(chat.ChatEvent) {})
	require.Error(t, err)
}

// TestLoadChat_AnswersWhileTheProjectionIsStillBehind pins the one property
// LoadChat exists for, and pins it as a PROPERTY rather than as a race.
//
// SetPlacement is deliberately on the async Send path (a single drag renumbers a
// whole level, and blocking each write on its projection would serialise the drag
// behind the read model). So between that write returning and the projection
// folding, agent_chats_read genuinely still holds the placement the chat had
// BEFORE — and that window is exactly when a create asks what the new chat
// inherits, microseconds later.
//
// Live, the window is too narrow to test against: a spawn that resolved placement
// from the read model, and therefore threaded nothing, passed about half the time
// and survived a suite that way. So the window is held open here instead of raced,
// by writing the pre-placement row back over the folded one. That is not a broken
// state being simulated — it is the ordinary state of this table for a moment
// after every placement, made to stand still long enough to assert against.
func TestLoadChat_AnswersWhileTheProjectionIsStillBehind(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.Create(ctx, chat.CreateInput{ID: "c2", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)

	// The read-model row as it stands BEFORE the placement: the exact bytes the
	// projection serves during the window.
	var unplaced []byte
	require.NoError(t,
		db.Raw("SELECT data FROM agent_chats_read WHERE id = ?", "c2").Row().Scan(&unplaced))

	_, err = repo.SetPlacement(ctx, "c2", "c1", 0)
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)
	require.NoError(t,
		db.Exec("UPDATE agent_chats_read SET data = ? WHERE id = ?", unplaced, "c2").Error)

	projected, err := repo.GetChat(ctx, "c2")
	require.NoError(t, err)
	require.Empty(t, projected.ParentID,
		"the fixture only means anything while the read model is behind — this is that state")

	loaded, err := repo.LoadChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c1", loaded.ParentID,
		"LoadChat folds from the event log, so a decision taken on it is never one taken on a stale projection")
	assert.Equal(t, 0, loaded.Order)
}

// TestSetOrder_RenumbersWithoutMoving pins the split at the repository boundary.
//
// The two placement writes exist because a drag decides a parent for ONE row and
// an index for a whole level. This is the second of them, and the guarantee is
// that it cannot express a parent at all: the aggregate applies it to the chat
// folded from the log, so the row keeps the parent the log has whatever the
// caller believed when it planned the renumber.
func TestSetOrder_RenumbersWithoutMoving(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.Create(ctx, chat.CreateInput{ID: "c2", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.SetPlacement(ctx, "c2", "c1", 0)
	require.NoError(t, err)

	renumbered, err := repo.SetOrder(ctx, "c2", 3)
	require.NoError(t, err)
	assert.Equal(t, "c1", renumbered.ParentID)
	assert.Equal(t, 3, renumbered.Order)

	loaded, err := repo.LoadChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c1", loaded.ParentID, "the chat is still threaded off the one it was filed under")
	assert.Equal(t, 3, loaded.Order)
}

func TestSetOrder_RefusesAChatThatDoesNotExist(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.SetOrder(ctx, "no-such-chat", 0)
	assert.Error(t, err)
}

// A miss arrives as this package's own sentinel whichever read served it, so no
// caller has to know that one folds from the log and the other reads a table.
func TestLoadChat_UnknownChatIsTheSameNotFoundGetChatReports(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.LoadChat(ctx, "no-such-chat")
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

// A read that FAILS is not a read that missed. Both paths funnel through the
// same not-found mapping, and a failure mapped to ErrNotFound would surface as a
// 404 — telling a user their chat no longer exists because a database was
// briefly unreadable, and inviting every caller to act on a deletion that never
// happened.
func TestGetChat_AReadFailureIsNotAMiss(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.GetChat(ctx, "c1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, chat.ErrNotFound,
		"a chat that cannot be READ is not a chat that is gone")
}
