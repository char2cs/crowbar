package agentrun

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
) asynx.Asynx[domain.AgentRun] {
	t.Helper()
	es, err := sqlite.NewEventStore(filepath.Join(dir, "agent_runs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })

	ax, err := asynx.New[domain.AgentRun]().
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

	token := uuid.NewString()
	run, err := repo.Create(ctx, "task-1", "brainstorming", token)
	require.NoError(t, err)

	assert.NotEmpty(t, run.ID)
	assert.Equal(t, "task-1", run.TaskID)
	assert.Equal(t, "brainstorming", run.StateName)
	assert.Equal(t, token, run.Token)
	assert.Equal(t, domain.AgentRunStatusRunning, run.Status)
	assert.False(t, run.CreatedAt.IsZero())
	assert.False(t, run.UpdatedAt.IsZero())

	// Token secondary index is populated — GetByToken must succeed.
	byToken, err := repo.GetByToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, run.ID, byToken.ID)
}

func TestCreate_ValidationError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	// Empty taskID — Validate rejects.
	_, err := repo.Create(ctx, "", "state", "token")
	assert.Error(t, err)

	// Empty stateName — Validate rejects.
	_, err = repo.Create(ctx, "task-1", "", "token")
	assert.Error(t, err)

	// Empty token — Validate rejects.
	_, err = repo.Create(ctx, "task-1", "state", "")
	assert.Error(t, err)
}

func TestGet(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	run, err := repo.Create(ctx, "task-1", "brainstorming", uuid.NewString())
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, got.ID)
	assert.Equal(t, run.TaskID, got.TaskID)
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

func TestGetByToken_Found(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	token := "tok-" + uuid.NewString()
	run, err := repo.Create(ctx, "task-2", "implementation", token)
	require.NoError(t, err)

	got, err := repo.GetByToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, run.ID, got.ID)
}

func TestGetByToken_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	_, err := repo.GetByToken(ctx, "unknown-token")
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
	run1, err := repo.Create(ctx, taskID, "state-a", uuid.NewString())
	require.NoError(t, err)
	run2, err := repo.Create(ctx, taskID, "state-b", uuid.NewString())
	require.NoError(t, err)
	// Run belonging to a different task — must not appear.
	_, err = repo.Create(ctx, "other-task", "state-a", uuid.NewString())
	require.NoError(t, err)

	list, err := repo.ListByTask(ctx, taskID)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	ids := map[string]bool{run1.ID: true, run2.ID: true}
	for _, r := range list {
		assert.True(t, ids[r.ID], "unexpected run %s in list", r.ID)
	}
}

func TestListByTask_Empty(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	list, err := repo.ListByTask(ctx, "no-runs-task")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCompleteAgentRun(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	run, err := repo.Create(ctx, "task-1", "brainstorming", uuid.NewString())
	require.NoError(t, err)

	err = repo.CompleteAgentRun(ctx, run.ID, "summary output")
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusCompleted, got.Status)
	require.Len(t, got.Outputs, 1)
	assert.Equal(t, "summary output", got.Outputs[0].Output)
	assert.Equal(t, "brainstorming", got.Outputs[0].StateName)
}

func TestCompleteAgentRun_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.CompleteAgentRun(ctx, "ghost-id", "output")
	assert.Error(t, err)
}

func TestFailAgentRun(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	run, err := repo.Create(ctx, "task-1", "brainstorming", uuid.NewString())
	require.NoError(t, err)

	err = repo.FailAgentRun(ctx, run.ID)
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusFailed, got.Status)
}

func TestFailAgentRun_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.FailAgentRun(ctx, "ghost-id")
	assert.Error(t, err)
}

func TestInterruptAgentRun(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	run, err := repo.Create(ctx, "task-1", "brainstorming", uuid.NewString())
	require.NoError(t, err)

	err = repo.InterruptAgentRun(ctx, run.ID)
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusInterrupted, got.Status)
}

func TestInterruptAgentRun_NotFound(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.InterruptAgentRun(ctx, "ghost-id")
	assert.Error(t, err)
}

// TestRecoverOrphanedRuns verifies that a run that was created but never
// completed (simulating a crash) is marked as failed by RecoverOrphanedRuns.
func TestRecoverOrphanedRuns(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	token := uuid.NewString()
	run, err := repo.Create(ctx, "task-crash", "brainstorming", token)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusRunning, run.Status)

	// Simulate crash — do NOT complete the run.
	// RecoverOrphanedRuns should detect it is still running and mark it failed.
	err = repo.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusFailed, got.Status)
}

// TestRecoverOrphanedRuns_AlreadyTerminal verifies that a completed run is
// left untouched by RecoverOrphanedRuns.
func TestRecoverOrphanedRuns_AlreadyTerminal(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	token := uuid.NewString()
	run, err := repo.Create(ctx, "task-terminal", "brainstorming", token)
	require.NoError(t, err)

	err = repo.CompleteAgentRun(ctx, run.ID, "done")
	require.NoError(t, err)

	err = repo.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)

	got, err := repo.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusCompleted, got.Status)
}

// TestRecoverOrphanedRuns_RebuildTokenIndex verifies that after recovery the
// token secondary index is rebuilt so GetByToken works without a prior Create.
func TestRecoverOrphanedRuns_RebuildTokenIndex(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)

	// Create a run via a fresh repo instance that populates the token index.
	repo1 := New(ax)
	token := "recover-tok-" + uuid.NewString()
	run, err := repo1.Create(ctx, "task-rebuild", "state-x", token)
	require.NoError(t, err)
	err = repo1.FailAgentRun(ctx, run.ID)
	require.NoError(t, err)

	// Simulate a process restart by creating a new repo wrapping the same ax.
	// The new repo has an empty tokenToID map; ids is empty too so RecoverOrphanedRuns
	// cannot rebuild unless we seed ids.
	// Because ids is populated during Create, the new repo has no knowledge of
	// existing runs. We seed it by copying ids from the old repo.
	repo2 := &agentRunRepo{ax: ax}
	repo2.mu.Lock()
	repo2.ids = append(repo2.ids, run.ID)
	repo2.mu.Unlock()

	err = repo2.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)

	// Token index should now be rebuilt.
	got, err := repo2.GetByToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, run.ID, got.ID)
}

// TestRecoverOrphanedRuns_Empty verifies no error when there are no runs.
func TestRecoverOrphanedRuns_Empty(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	err := repo.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)
}

func TestSubscribe(
	t *testing.T,
) {
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	id, err := repo.Subscribe("agent_run.*", func(
		_ context.Context,
		_ asynxModels.Event[domain.AgentRun],
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

	_, err := repo.Subscribe("agent_run.*", func(
		_ context.Context,
		_ asynxModels.Event[domain.AgentRun],
	) {})
	assert.Error(t, err)
}

func TestListByTask_GetError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)

	repo := &agentRunRepo{ax: ax}
	repo.mu.Lock()
	repo.ids = append(repo.ids, "ghost-id-that-does-not-exist")
	repo.mu.Unlock()

	list, err := repo.ListByTask(ctx, "any-task")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRecoverOrphanedRuns_GetError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)

	repo := &agentRunRepo{ax: ax}
	repo.mu.Lock()
	repo.ids = append(repo.ids, "ghost-id-recover")
	repo.mu.Unlock()

	err := repo.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)
}

func TestRecoverOrphanedRuns_FailError(
	t *testing.T,
) {
	ctx := context.Background()
	dir := t.TempDir()
	ax := newTestAsynx(t, dir)
	repo := New(ax)

	token := uuid.NewString()
	run, err := repo.Create(ctx, "task-crash-fail", "brainstorming", token)
	require.NoError(t, err)

	require.NoError(t, ax.Shutdown(context.Background()))

	inner := repo.(*agentRunRepo)
	err = inner.RecoverOrphanedRuns(ctx)
	require.NoError(t, err)

	_ = run
}
