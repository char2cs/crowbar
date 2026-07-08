package reviewthread_test

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
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	rtcmds "github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// captureBroadcast records every frame the hub projection fans out so tests can
// assert the WS frame shape (spec §3.5 delivery — the reviewthread hub frame is
// the bare evt.Aggregate, no enrichment).
type captureBroadcast struct {
	mu   sync.Mutex
	rows []domain.ReviewThread
}

func (c *captureBroadcast) push(th domain.ReviewThread) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, th)
}

func (c *captureBroadcast) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}

func (c *captureBroadcast) last() domain.ReviewThread {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rows[len(c.rows)-1]
}

// newRepoWithDeps builds a reviewthread repo over throwaway in-memory event +
// read-model DBs (the central per-type stores' shape) and returns the read-model
// DB and the captured hub broadcast so tests can inspect persistence + fan-out.
func newRepoWithDeps(
	t *testing.T,
) (context.Context, reviewthread.ReviewThread, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	cap := &captureBroadcast{}
	repo, err := reviewthread.New(ax, es, db, cap.push)
	require.NoError(t, err)
	return context.Background(), repo, db, cap
}

func newRepo(
	t *testing.T,
) (context.Context, reviewthread.ReviewThread) {
	t.Helper()
	ctx, repo, _, _ := newRepoWithDeps(t)
	return ctx, repo
}

func TestReviewThread_OpenResolveReopen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1",
	}, time.Unix(1, 0))
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
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w1", MessageID: "m2"}, time.Unix(1, 0))
	assert.Error(t, err)
	// A duplicate Open is a state-guard rejection (ErrValidation), never retried.
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
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
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t3", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, "t3")
	assert.Error(t, err)
}

func TestReviewThread_Reopen_ErrorWhenOpen(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t4", WsID: "w1", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	_, err = repo.Reopen(ctx, "t4")
	assert.Error(t, err)
}

func TestReviewThread_Get_ReturnsOpened(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t5", WsID: "w2", MessageID: "m1"}, time.Unix(1, 0))
	require.NoError(t, err)
	got, err := repo.Get(ctx, "t5")
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusOpen, got.Status)
	assert.Equal(t, "w2", got.WsID)
}

func TestReviewThread_OpenReplyList(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "b.go", Body: "first",
	}, now)
	require.NoError(t, err)

	replied, err := repo.Reply(ctx, "t1", "m2", "", false, "second", now)
	require.NoError(t, err)
	require.Len(t, replied.Messages, 2)
	assert.Equal(t, "second", replied.Messages[1].Body)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	byWs, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, byWs, 1)
	assert.Equal(t, "t1", byWs[0].ID)
}

func TestReviewThread_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w2", MessageID: "m2"}, now)
	require.NoError(t, err)

	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "t1", list[0].ID)

	none, err := repo.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestReviewThread_Reply_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Reply(ctx, "no-thread", "m1", "", false, "body", time.Unix(1, 0))
	assert.Error(t, err)
}

func TestReviewThread_DeleteThread_Forgets(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteThread(ctx, "t1"))

	// Forget runs the store projection's synchronous OnForget row-delete, so the
	// read-model row is gone the instant Forget returns (spec §3.6).
	var n int64
	require.NoError(t, db.WithContext(ctx).Table("read_review_threads").Count(&n).Error)
	assert.Zero(t, n)

	_, err = repo.Get(ctx, "t1")
	assert.Error(t, err)
}

// TestReviewThread_ConcurrentReplies_NoWriteMu_OCC proves the writeMu deletion
// (decision 10) for reviewthread: many replies targeting the SAME thread at once
// must ALL commit via Send + OCC retry (shard routing + (id,version) uniqueness +
// bounded retry), with no hand-rolled serialize.KeyedMutex. Run with -race.
func TestReviewThread_ConcurrentReplies_NoWriteMu_OCC(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m0", Body: "first",
	}, now)
	require.NoError(t, err)

	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.Reply(ctx, "t1", fmt.Sprintf("m-%d", idx), "", false, "reply", now)
		}(i)
	}
	wg.Wait()

	failed := 0
	for _, e := range errs {
		if e != nil {
			failed++
		}
	}
	require.Zero(t, failed, "%d/%d concurrent same-aggregate replies failed under OCC (no writeMu)", failed, n)

	// The aggregate folds cleanly from the log after the storm.
	got, err := repo.Get(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, got.Messages, n+1)
}

// TestReviewThread_StoreProjectionPersists proves the save-only store projection
// folds evt.Aggregate into the durable read model (state/store/review_thread.db).
func TestReviewThread_StoreProjectionPersists(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go",
	}, time.Unix(1, 0))
	require.NoError(t, err)

	// The store projection runs async after the event is durable; drain it, then
	// assert synchronously that it persisted the row to the read-model DB.
	reviewthread.WaitQuiescentForTest(repo)
	var n int64
	require.NoError(t, db.WithContext(ctx).Table("read_review_threads").Count(&n).Error)
	require.Equal(t, int64(1), n, "store projection must persist the row to state/store/review_thread.db")
}

// TestReviewThread_ListLazyRebuild proves the lazy Replay repair (spec §3.7): when
// the durable read model is lost but the event log survives, the per-request List
// enumerates every id via the event store's AggregateLister and Replays each back
// into the read model.
func TestReviewThread_ListLazyRebuild(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w2", MessageID: "m2"}, now)
	require.NoError(t, err)

	// The store projection runs async; drain it so both rows are durable before
	// simulating a loss, then read synchronously so the assertion is deterministic.
	reviewthread.WaitQuiescentForTest(repo)
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// Simulate read-model loss with the event log intact.
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_review_threads").Error)

	// The per-request List heals the model via whole-model lazy Replay.
	rebuilt, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, rebuilt, 2, "List must Replay every id from the event log after a read-model loss")
	got := []string{rebuilt[0].ID, rebuilt[1].ID}
	require.ElementsMatch(t, []string{"t1", "t2"}, got)
}

// TestReviewThread_HubBroadcastsBareAggregate proves the split hub projection fans
// out the unchanged frame shape — the bare evt.Aggregate (no enrichment) — so the
// FE review-thread stream sees no regression (spec §3.5 delivery, Step 1 case d).
func TestReviewThread_HubBroadcastsBareAggregate(t *testing.T) {
	ctx, repo, _, cap := newRepoWithDeps(t)
	opened, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", Body: "first",
	}, time.Unix(1, 0))
	require.NoError(t, err)

	// The hub projection runs async; drain it, then assert the broadcast count
	// synchronously — no polling, no timeout.
	reviewthread.WaitQuiescentForTest(repo)
	require.GreaterOrEqual(t, cap.count(), 1, "hub projection must broadcast the review-thread frame")
	frame := cap.last()
	assert.Equal(t, "t1", frame.ID)
	assert.Equal(t, "w1", frame.WsID)
	assert.Equal(t, "a.go", frame.FilePath)
	assert.Equal(t, opened.Status, frame.Status)
	require.Len(t, frame.Messages, 1)
	assert.Equal(t, "first", frame.Messages[0].Body)
}

// TestReviewThread_OccSendErrorDisposition pins the terminal error-disposition
// contract (spec §3.5, decision 10) against a fake send: ErrPipelineFailed is
// retried exactly MaxOCCAttempts then surfaced (→ 409); ErrValidation is never
// retried (→ 422); ErrQueueFull is never retried and is translated to
// apperr.ErrUnavailable (→ 503). All classified via errors.Is.
func TestReviewThread_OccSendErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := rtcmds.ResolveReviewThread{ID: "t1"}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.ReviewThread]) (asynxModels.Event[domain.ReviewThread], error) {
			calls++
			return asynxModels.Event[domain.ReviewThread]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := reviewthread.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, reviewthread.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.ReviewThread]) (asynxModels.Event[domain.ReviewThread], error) {
			calls++
			return asynxModels.Event[domain.ReviewThread]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := reviewthread.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.ReviewThread]) (asynxModels.Event[domain.ReviewThread], error) {
			calls++
			return asynxModels.Event[domain.ReviewThread]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := reviewthread.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.ReviewThread]) (asynxModels.Event[domain.ReviewThread], error) {
			return asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: c.AggregateID()}}, nil
		}
		evt, err := reviewthread.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "t1", evt.Aggregate.ID)
	})
}

func TestReviewThread_List_StorageError(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.List(ctx)
	assert.Error(t, err)
}

func TestReviewThread_ListByWorkspace_StorageError(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.ListByWorkspace(ctx, "w1")
	assert.Error(t, err)
}

func TestReviewThread_New_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
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

	_, err = reviewthread.New(ax, es, db, func(domain.ReviewThread) {})
	assert.Error(t, err)
}
