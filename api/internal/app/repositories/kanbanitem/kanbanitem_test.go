package kanbanitem

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
) asynx.Asynx[domain.KanbanItem] {
	t.Helper()
	es, err := sqlite.NewEventStore(filepath.Join(dir, "kanban_items.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })

	ax, err := asynx.New[domain.KanbanItem]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	return ax
}

func TestNew(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)
	assert.NotNil(t, repo)
}

func TestCreate(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-1", "Implement login", "run-abc")
	require.NoError(t, err)

	assert.NotEmpty(t, item.ID)
	assert.Equal(t, "task-1", item.TaskID)
	assert.Equal(t, "Implement login", item.Title)
	assert.Equal(t, "open", item.Status)
	assert.Equal(t, "run-abc", item.AgentRunID)
	assert.False(t, item.CreatedAt.IsZero())
	assert.False(t, item.UpdatedAt.IsZero())
}

func TestCreate_WithoutAgentRun(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-2", "Write tests", "")
	require.NoError(t, err)
	assert.Equal(t, "", item.AgentRunID)
}

func TestCreate_ValidationError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	// Empty taskID — Validate rejects.
	_, err := repo.Create(ctx, "", "title", "")
	assert.Error(t, err)

	// Empty title — Validate rejects.
	_, err = repo.Create(ctx, "task-1", "", "")
	assert.Error(t, err)
}

func TestGet(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-1", "Get test", "")
	require.NoError(t, err)

	got, err := repo.Get(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
	assert.Equal(t, item.Title, got.Title)
	assert.Equal(t, item.TaskID, got.TaskID)
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

func TestListByTask(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	taskID := "task-list-" + uuid.NewString()
	item1, err := repo.Create(ctx, taskID, "Item A", "")
	require.NoError(t, err)
	item2, err := repo.Create(ctx, taskID, "Item B", "run-1")
	require.NoError(t, err)
	// Item belonging to another task — must not appear.
	_, err = repo.Create(ctx, "other-task", "Other Item", "")
	require.NoError(t, err)

	list, err := repo.ListByTask(ctx, taskID)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	ids := map[string]bool{item1.ID: true, item2.ID: true}
	for _, it := range list {
		assert.True(t, ids[it.ID], "unexpected item %s in list", it.ID)
	}
}

func TestListByTask_Empty(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	list, err := repo.ListByTask(ctx, "no-items-task")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUpdateStatus(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-1", "Update Status Test", "")
	require.NoError(t, err)
	assert.Equal(t, "open", item.Status)

	err = repo.UpdateStatus(ctx, item.ID, "in_progress", "run-x")
	require.NoError(t, err)

	got, err := repo.Get(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "in_progress", got.Status)
	assert.Equal(t, "run-x", got.AgentRunID)
}

func TestUpdateStatus_ClearAgentRunID(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-1", "Clear AgentRun Test", "run-abc")
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, item.ID, "done", "")
	require.NoError(t, err)

	got, err := repo.Get(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "done", got.Status)
	assert.Equal(t, "", got.AgentRunID)
}

func TestUpdateStatus_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.UpdateStatus(ctx, "ghost-id", "open", "")
	assert.Error(t, err)
}

func TestUpdateStatus_EmptyStatus(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	item, err := repo.Create(ctx, "task-1", "Empty Status Test", "")
	require.NoError(t, err)

	// Empty status — Validate rejects.
	err = repo.UpdateStatus(ctx, item.ID, "", "")
	assert.Error(t, err)
}

func TestSubscribe(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	id, err := repo.Subscribe("kanban_item.*", func(
		_ context.Context,
		_ asynxModels.Event[domain.KanbanItem],
	) {})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}

func TestSubscribe_Error(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	require.NoError(t, ax.Shutdown(context.Background()))
	repo := New(ax)

	_, err := repo.Subscribe("kanban_item.*", func(
		_ context.Context,
		_ asynxModels.Event[domain.KanbanItem],
	) {})
	assert.Error(t, err)
}

func TestListByTask_GetError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)

	repo := &kanbanItemRepo{ax: ax}
	repo.mu.Lock()
	repo.ids = append(repo.ids, "ghost-id-that-does-not-exist")
	repo.mu.Unlock()

	list, err := repo.ListByTask(ctx, "any-task")
	require.NoError(t, err)
	assert.Empty(t, list)
}
