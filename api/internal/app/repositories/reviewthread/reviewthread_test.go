package reviewthread

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newTestAsynx opens a fresh SQLite event store in dir and returns an Asynx instance.
func newTestAsynx(
	t *testing.T,
	dir string,
) asynx.Asynx[domain.ReviewThread] {
	t.Helper()
	es, err := sqlite.NewEventStore(filepath.Join(dir, "review_threads.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })

	ax, err := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	return ax
}

func strPtr(
	s string,
) *string {
	return &s
}

func TestNew(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)
	assert.NotNil(t, repo)
}

func TestOpen(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	agentRunID := strPtr("run-1")
	thread, err := repo.Open(
		ctx,
		"task-1",
		agentRunID,
		"src/main.go",
		42,
		domain.ReviewPhaseAIReview,
		"reviewer",
		"This needs a refactor.",
	)
	require.NoError(t, err)

	assert.NotEmpty(t, thread.ID)
	assert.Equal(t, "task-1", thread.TaskID)
	assert.Equal(t, agentRunID, thread.AgentRunID)
	assert.Equal(t, "src/main.go", thread.File)
	assert.Equal(t, 42, thread.Line)
	assert.Equal(t, domain.ReviewPhaseAIReview, thread.Phase)
	assert.Equal(t, "reviewer", thread.OpenedBy)
	assert.Equal(t, domain.ReviewThreadStatusOpen, thread.Status)
	assert.False(t, thread.CreatedAt.IsZero())
	assert.False(t, thread.UpdatedAt.IsZero())

	// First message is appended.
	require.Len(t, thread.Messages, 1)
	assert.Equal(t, "reviewer", thread.Messages[0].Role)
	assert.Equal(t, "This needs a refactor.", thread.Messages[0].Content)
	assert.Equal(t, thread.ID, thread.Messages[0].ThreadID)
	assert.NotEmpty(t, thread.Messages[0].ID)
	assert.False(t, thread.Messages[0].CreatedAt.IsZero())
}

func TestOpen_WithoutAgentRun(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(
		ctx,
		"task-2",
		nil,
		"cmd/main.go",
		1,
		domain.ReviewPhaseHumanReview,
		"human",
		"Looks good.",
	)
	require.NoError(t, err)
	assert.Nil(t, thread.AgentRunID)
}

func TestOpen_ValidationError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	// Empty taskID — Validate rejects.
	_, err := repo.Open(ctx, "", nil, "file.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	assert.Error(t, err)

	// Empty file — Validate rejects.
	_, err = repo.Open(ctx, "task-1", nil, "", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	assert.Error(t, err)

	// Empty openedBy — Validate rejects.
	_, err = repo.Open(ctx, "task-1", nil, "file.go", 1, domain.ReviewPhaseAIReview, "", "msg")
	assert.Error(t, err)
}

func TestGet(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 10, domain.ReviewPhaseAIReview, "reviewer", "hello")
	require.NoError(t, err)

	got, err := repo.Get(ctx, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, thread.ID, got.ID)
}

func TestGet_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	_, err := repo.Get(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestListByTask_NoFilter(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	taskID := "task-list-" + uuid.NewString()
	t1, err := repo.Open(ctx, taskID, nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg-1")
	require.NoError(t, err)
	t2, err := repo.Open(ctx, taskID, nil, "b.go", 2, domain.ReviewPhaseHumanReview, "human", "msg-2")
	require.NoError(t, err)
	// Different task — should not appear.
	_, err = repo.Open(ctx, "other-task", nil, "c.go", 3, domain.ReviewPhaseAIReview, "reviewer", "msg-3")
	require.NoError(t, err)

	list, err := repo.ListByTask(ctx, taskID, "", "")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	ids := map[string]bool{t1.ID: true, t2.ID: true}
	for _, th := range list {
		assert.True(t, ids[th.ID], "unexpected thread %s", th.ID)
	}
}

func TestListByTask_FilterByStatus(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	taskID := "task-status-filter-" + uuid.NewString()
	open1, err := repo.Open(ctx, taskID, nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)
	open2, err := repo.Open(ctx, taskID, nil, "b.go", 2, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)

	// Resolve one thread.
	err = repo.ResolveThread(ctx, open2.ID, ":thumbsup:")
	require.NoError(t, err)

	// Filter by "open" — only open1.
	list, err := repo.ListByTask(ctx, taskID, string(domain.ReviewThreadStatusOpen), "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, open1.ID, list[0].ID)

	// Filter by "agreed" — only open2.
	list, err = repo.ListByTask(ctx, taskID, string(domain.ReviewThreadStatusAgreed), "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, open2.ID, list[0].ID)
}

func TestListByTask_FilterByPhase(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	taskID := "task-phase-filter-" + uuid.NewString()
	ai, err := repo.Open(ctx, taskID, nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)
	_, err = repo.Open(ctx, taskID, nil, "b.go", 2, domain.ReviewPhaseHumanReview, "human", "msg")
	require.NoError(t, err)

	list, err := repo.ListByTask(ctx, taskID, "", string(domain.ReviewPhaseAIReview))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ai.ID, list[0].ID)
}

func TestListByTask_FilterByStatusAndPhase(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	taskID := "task-both-filter-" + uuid.NewString()
	target, err := repo.Open(ctx, taskID, nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)
	_, err = repo.Open(ctx, taskID, nil, "b.go", 2, domain.ReviewPhaseHumanReview, "human", "msg")
	require.NoError(t, err)

	list, err := repo.ListByTask(
		ctx,
		taskID,
		string(domain.ReviewThreadStatusOpen),
		string(domain.ReviewPhaseAIReview),
	)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, target.ID, list[0].ID)
}

func TestListByTask_Empty(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	list, err := repo.ListByTask(ctx, "empty-task", "", "")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestPostMessage(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "first")
	require.NoError(t, err)

	err = repo.PostMessage(ctx, thread.ID, "implementer", "I fixed it.")
	require.NoError(t, err)

	got, err := repo.Get(ctx, thread.ID)
	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "implementer", got.Messages[1].Role)
	assert.Equal(t, "I fixed it.", got.Messages[1].Content)
	assert.Equal(t, thread.ID, got.Messages[1].ThreadID)
	assert.NotEmpty(t, got.Messages[1].ID)
}

func TestPostMessage_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.PostMessage(ctx, "ghost-thread", "implementer", "content")
	assert.Error(t, err)
}

func TestPostMessage_ValidationError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)

	// Empty role — Validate rejects.
	err = repo.PostMessage(ctx, thread.ID, "", "content")
	assert.Error(t, err)

	// Empty content — Validate rejects.
	err = repo.PostMessage(ctx, thread.ID, "role", "")
	assert.Error(t, err)
}

func TestForceApprove(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)

	err = repo.ForceApprove(ctx, thread.ID)
	require.NoError(t, err)

	got, err := repo.Get(ctx, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusForceApproved, got.Status)
}

func TestForceApprove_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.ForceApprove(ctx, "ghost-thread")
	assert.Error(t, err)
}

func TestResolveThread(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)

	err = repo.ResolveThread(ctx, thread.ID, ":white_check_mark:")
	require.NoError(t, err)

	got, err := repo.Get(ctx, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusAgreed, got.Status)
	assert.Equal(t, ":white_check_mark:", got.Emoji)
}

func TestResolveThread_NoEmoji(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	thread, err := repo.Open(ctx, "task-1", nil, "a.go", 1, domain.ReviewPhaseAIReview, "reviewer", "msg")
	require.NoError(t, err)

	err = repo.ResolveThread(ctx, thread.ID, "")
	require.NoError(t, err)

	got, err := repo.Get(ctx, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReviewThreadStatusAgreed, got.Status)
	assert.Equal(t, "", got.Emoji)
}

func TestResolveThread_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.ResolveThread(ctx, "ghost-thread", ":fire:")
	assert.Error(t, err)
}

func TestSubscribe(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	id, err := repo.Subscribe("review_thread.*", func(
		_ context.Context,
		_ asynxModels.Event[domain.ReviewThread],
	) {})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}
