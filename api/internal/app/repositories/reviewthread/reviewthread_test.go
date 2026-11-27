package reviewthread_test

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
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

func newRepoWithDeps(
	t *testing.T,
) (context.Context, reviewthread.ReviewThread, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rm, err := reviewthread.OpenReadModel(db)
	require.NoError(t, err)
	b := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	reviewthread.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	cap := &captureBroadcast{}
	repo, err := reviewthread.New(ax, es, rm, cap.push)
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
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
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
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "b.go", LineNumber: 1, Body: "first",
	}, now)
	require.NoError(t, err)

	replied, err := repo.Reply(ctx, reviewthread.ReplyInput{ID: "t1", MessageID: "m2", Body: "second"}, now)
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

func TestReviewThread_AttributionSurvivesTheEventLog(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "b.go", LineNumber: 1, Body: "finding",
		Author: "claude", IsAgent: true, ProviderID: "claude", ChatID: "chat-1",
	}, now)
	require.NoError(t, err)
	_, err = repo.Reply(ctx, reviewthread.ReplyInput{
		ID: "t1", MessageID: "m2", Body: "addressed",
		Author: "codex", IsAgent: true, ProviderID: "codex", ChatID: "chat-2",
	}, now)
	require.NoError(t, err)
	_, err = repo.Reply(ctx, reviewthread.ReplyInput{ID: "t1", MessageID: "m3", Body: "thanks"}, now)
	require.NoError(t, err)

	got, err := repo.Get(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	assert.Equal(t, "claude", got.Messages[0].ProviderID)
	assert.Equal(t, "chat-1", got.Messages[0].ChatID)
	assert.Equal(t, "codex", got.Messages[1].ProviderID)
	assert.Equal(t, "chat-2", got.Messages[1].ChatID)
	assert.Empty(t, got.Messages[2].ProviderID)
	assert.Empty(t, got.Messages[2].ChatID)

	reviewthread.WaitQuiescentForTest(repo)
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Len(t, list[0].Messages, 3)
	assert.Equal(t, "claude", list[0].Messages[0].ProviderID)
	assert.Equal(t, "chat-2", list[0].Messages[1].ChatID)
	assert.Empty(t, list[0].Messages[2].ProviderID)
}

func TestReviewThread_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w2", MessageID: "m2"}, now)
	require.NoError(t, err)
	reviewthread.WaitQuiescentForTest(repo)
	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "t1", list[0].ID)

	none, err := repo.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestReviewThread_AnUnknownThreadIsNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	verbs := map[string]func() error{
		"get": func() error { _, err := repo.Get(ctx, "ghost"); return err },
		"reply": func() error {
			_, err := repo.Reply(ctx, reviewthread.ReplyInput{ID: "ghost", MessageID: "m1", Body: "body"}, time.Unix(1, 0))
			return err
		},
		"edit":           func() error { _, err := repo.EditMessage(ctx, "ghost", "m1", "x"); return err },
		"delete message": func() error { _, err := repo.DeleteMessage(ctx, "ghost", "m1"); return err },
		"resolve":        func() error { _, err := repo.Resolve(ctx, "ghost"); return err },
		"reopen":         func() error { _, err := repo.Reopen(ctx, "ghost"); return err },
		"delete thread":  func() error { return repo.DeleteThread(ctx, "ghost") },
	}
	for verb, run := range verbs {
		assert.Equal(t, problem.Problem{Code: problem.NotFound, Message: "review thread not found"}, problem.Of(run()), verb)
	}
}

func TestReviewThread_DeleteThread_Forgets(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteThread(ctx, "t1"))

	var n int64
	require.NoError(t, db.WithContext(ctx).Table("read_review_threads").Count(&n).Error)
	assert.Zero(t, n)

	_, err = repo.Get(ctx, "t1")
	assert.Error(t, err)
}

func TestReviewThread_DeleteThread_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)

	err := repo.DeleteThread(ctx, "does-not-exist")

	assert.Error(t, err, "forgetting an aggregate that was never opened is a caller bug, not a no-op")
}

func TestReviewThread_ConcurrentRepliesAllCommitInTurn(t *testing.T) {
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
			_, errs[idx] = repo.Reply(ctx, reviewthread.ReplyInput{
				ID: "t1", MessageID: fmt.Sprintf("m-%d", idx), Body: "reply",
			}, now)
		}(i)
	}
	wg.Wait()

	failed := 0
	for _, e := range errs {
		if e != nil {
			failed++
		}
	}
	require.Zero(t, failed, "%d/%d concurrent same-aggregate replies failed", failed, n)

	got, err := repo.Get(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, got.Messages, n+1)
}

func TestReviewThread_StoreProjectionPersists(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", LineNumber: 1,
	}, time.Unix(1, 0))
	require.NoError(t, err)

	reviewthread.WaitQuiescentForTest(repo)
	var n int64
	require.NoError(t, db.WithContext(ctx).Table("read_review_threads").Count(&n).Error)
	require.Equal(t, int64(1), n, "store projection must persist the row to state/store/review_thread.db")
}

func TestReviewThread_ListLazyRebuild(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1"}, now)
	require.NoError(t, err)
	_, err = repo.Open(ctx, reviewthread.OpenInput{ID: "t2", WsID: "w2", MessageID: "m2"}, now)
	require.NoError(t, err)

	reviewthread.WaitQuiescentForTest(repo)
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM read_review_threads").Error)

	rebuilt, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, rebuilt, 2, "List must Replay every id from the event log after a read-model loss")
	got := []string{rebuilt[0].ID, rebuilt[1].ID}
	require.ElementsMatch(t, []string{"t1", "t2"}, got)
}

func TestReviewThread_HubBroadcastsBareAggregate(t *testing.T) {
	ctx, repo, _, cap := newRepoWithDeps(t)
	opened, err := repo.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", LineNumber: 1, Body: "first",
	}, time.Unix(1, 0))
	require.NoError(t, err)

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

func TestReviewThread_EditMessage_UpdatesBody(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1", Body: "first"}, now)
	require.NoError(t, err)
	_, err = repo.Reply(ctx, reviewthread.ReplyInput{ID: "t1", MessageID: "m2", Body: "second"}, now)
	require.NoError(t, err)

	edited, err := repo.EditMessage(ctx, "t1", "m2", "second, corrected")

	require.NoError(t, err)
	require.Len(t, edited.Messages, 2)
	assert.Equal(t, "first", edited.Messages[0].Body, "editing m2 must not touch m1")
	assert.Equal(t, "second, corrected", edited.Messages[1].Body)
}

func TestReviewThread_EditMessage_ErrorOnMissingThread(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.EditMessage(ctx, "does-not-exist", "m1", "x")

	assert.Error(t, err)
}

func TestReviewThread_EditMessage_ErrorOnUnknownMessageID(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1", Body: "first"}, time.Unix(1, 0))
	require.NoError(t, err)

	_, err = repo.EditMessage(ctx, "t1", "no-such-message", "x")

	assert.Error(t, err)
}

func TestReviewThread_DeleteMessage_RemovesAReply(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1", Body: "first"}, now)
	require.NoError(t, err)
	_, err = repo.Reply(ctx, reviewthread.ReplyInput{ID: "t1", MessageID: "m2", Body: "second"}, now)
	require.NoError(t, err)

	deleted, err := repo.DeleteMessage(ctx, "t1", "m2")

	require.NoError(t, err)
	require.Len(t, deleted.Messages, 1)
	assert.Equal(t, "m1", deleted.Messages[0].ID)
}

func TestReviewThread_DeleteMessage_ErrorOnMissingThread(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.DeleteMessage(ctx, "does-not-exist", "m1")

	assert.Error(t, err)
}

func TestReviewThread_DeleteMessage_ErrorOnRootComment(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Open(ctx, reviewthread.OpenInput{ID: "t1", WsID: "w1", MessageID: "m1", Body: "first"}, time.Unix(1, 0))
	require.NoError(t, err)

	_, err = repo.DeleteMessage(ctx, "t1", "m1")

	assert.Error(t, err)
}

func TestReviewThread_OpenReadModel_ErrorOnBadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = reviewthread.OpenReadModel(db)
	require.Error(t, err)
}
