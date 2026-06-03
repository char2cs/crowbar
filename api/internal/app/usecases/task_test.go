package usecases

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

func builtinLoader() flow.Loader {
	return &mockTaskFlowLoader{
		loadFn: func(flowPath string) (flow.FlowDefinition, error) {
			return flow.FlowDefinition{
				States: []flow.StateDefinition{{Name: "planning"}},
			}, nil
		},
	}
}

func TestTaskCreate_Happy(t *testing.T) {
	want := domain.Task{ID: "t1", RepositoryID: "r1"}
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{ID: id, Path: "/tmp/repo"}, nil
		},
	}
	taskRepo := &mockTaskTaskRepo{
		createFn: func(_ context.Context, repositoryID, title, flowPath, initialState, branchName, baseBranch, worktreePath string) (domain.Task, error) {
			return want, nil
		},
	}
	uc := NewTask(taskRepo, repoRepo, builtinLoader())
	got, err := uc.Create(context.Background(), "r1", "my task", "feat/x", "main", "")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestTaskCreate_WorktreePathDerived(t *testing.T) {
	var capturedWorktree string
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{ID: id, Path: "/code/myrepo"}, nil
		},
	}
	taskRepo := &mockTaskTaskRepo{
		createFn: func(_ context.Context, _, _, _, _, _, _, worktreePath string) (domain.Task, error) {
			capturedWorktree = worktreePath
			return domain.Task{}, nil
		},
	}
	uc := NewTask(taskRepo, repoRepo, builtinLoader())
	_, err := uc.Create(context.Background(), "r1", "task", "feat/y", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "/code/myrepo/.worktrees/feat/y", capturedWorktree)
}

func TestTaskCreate_RepositoryNotFound(t *testing.T) {
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{}, domain.ErrNotFound
		},
	}
	uc := NewTask(&mockTaskTaskRepo{}, repoRepo, builtinLoader())
	_, err := uc.Create(context.Background(), "missing", "task", "b", "main", "")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTaskCreate_RepoGetError(t *testing.T) {
	boom := errors.New("boom")
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{}, boom
		},
	}
	uc := NewTask(&mockTaskTaskRepo{}, repoRepo, builtinLoader())
	_, err := uc.Create(context.Background(), "r1", "task", "b", "main", "")
	assert.ErrorIs(t, err, boom)
}

func TestTaskGet_Delegates(t *testing.T) {
	want := domain.Task{ID: "t1"}
	taskRepo := &mockTaskTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return want, nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	got, err := uc.Get(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestTaskGet_PropagatesError(t *testing.T) {
	taskRepo := &mockTaskTaskRepo{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, domain.ErrNotFound
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	_, err := uc.Get(context.Background(), "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTaskArchive_Delegates(t *testing.T) {
	var called string
	taskRepo := &mockTaskTaskRepo{
		archiveFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	require.NoError(t, uc.Archive(context.Background(), "t1"))
	assert.Equal(t, "t1", called)
}

func TestTaskPause_Delegates(t *testing.T) {
	var called string
	taskRepo := &mockTaskTaskRepo{
		pauseFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	require.NoError(t, uc.Pause(context.Background(), "t1"))
	assert.Equal(t, "t1", called)
}

func TestTaskResume_Delegates(t *testing.T) {
	var called string
	taskRepo := &mockTaskTaskRepo{
		resumeFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	require.NoError(t, uc.Resume(context.Background(), "t1"))
	assert.Equal(t, "t1", called)
}

func TestTaskRetry_Delegates(t *testing.T) {
	var called string
	taskRepo := &mockTaskTaskRepo{
		retryFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	require.NoError(t, uc.Retry(context.Background(), "t1"))
	assert.Equal(t, "t1", called)
}

func TestTaskForceTransition_Delegates(t *testing.T) {
	var capturedID, capturedState string
	taskRepo := &mockTaskTaskRepo{
		forceTransitionFn: func(_ context.Context, id, state string) error {
			capturedID = id
			capturedState = state
			return nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	require.NoError(t, uc.ForceTransition(context.Background(), "t1", "planning"))
	assert.Equal(t, "t1", capturedID)
	assert.Equal(t, "planning", capturedState)
}

// ---------- local mocks (prefixed to avoid collision with project_test mocks) ----------

type mockTaskRepositoryRepo struct {
	getFn func(ctx context.Context, id string) (domain.Repository, error)
}

func (m *mockTaskRepositoryRepo) Create(_ context.Context, _, _, _ string) (domain.Repository, error) {
	return domain.Repository{}, nil
}
func (m *mockTaskRepositoryRepo) List(_ context.Context, _ string) ([]domain.Repository, error) {
	return nil, nil
}
func (m *mockTaskRepositoryRepo) Get(ctx context.Context, id string) (domain.Repository, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.Repository{}, nil
}
func (m *mockTaskRepositoryRepo) Delete(_ context.Context, _ string) error { return nil }

type mockTaskTaskRepo struct {
	createFn          func(ctx context.Context, repositoryID, title, flowPath, initialState, branchName, baseBranch, worktreePath string) (domain.Task, error)
	listFn            func(ctx context.Context, repositoryID string) ([]domain.Task, error)
	getFn             func(ctx context.Context, id string) (domain.Task, error)
	archiveFn         func(ctx context.Context, id string) error
	pauseFn           func(ctx context.Context, id string) error
	resumeFn          func(ctx context.Context, id string) error
	retryFn           func(ctx context.Context, id string) error
	advanceStateFn    func(ctx context.Context, id, nextState, event string) error
	forceTransitionFn func(ctx context.Context, id, toState string) error
}

func (m *mockTaskTaskRepo) Create(ctx context.Context, repositoryID, title, flowPath, initialState, branchName, baseBranch, repoPath, worktreePath string) (domain.Task, error) {
	if m.createFn != nil {
		return m.createFn(ctx, repositoryID, title, flowPath, initialState, branchName, baseBranch, worktreePath)
	}
	return domain.Task{}, nil
}

func (m *mockTaskTaskRepo) List(ctx context.Context, repositoryID string) ([]domain.Task, error) {
	if m.listFn != nil {
		return m.listFn(ctx, repositoryID)
	}
	return nil, nil
}
func (m *mockTaskTaskRepo) Get(ctx context.Context, id string) (domain.Task, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.Task{}, nil
}
func (m *mockTaskTaskRepo) Archive(ctx context.Context, id string) error {
	if m.archiveFn != nil {
		return m.archiveFn(ctx, id)
	}
	return nil
}
func (m *mockTaskTaskRepo) Pause(ctx context.Context, id string) error {
	if m.pauseFn != nil {
		return m.pauseFn(ctx, id)
	}
	return nil
}
func (m *mockTaskTaskRepo) Resume(ctx context.Context, id string) error {
	if m.resumeFn != nil {
		return m.resumeFn(ctx, id)
	}
	return nil
}
func (m *mockTaskTaskRepo) Retry(ctx context.Context, id string) error {
	if m.retryFn != nil {
		return m.retryFn(ctx, id)
	}
	return nil
}
func (m *mockTaskTaskRepo) AdvanceState(ctx context.Context, id, nextState, event string) error {
	if m.advanceStateFn != nil {
		return m.advanceStateFn(ctx, id, nextState, event)
	}
	return nil
}
func (m *mockTaskTaskRepo) ForceTransition(ctx context.Context, id, toState string) error {
	if m.forceTransitionFn != nil {
		return m.forceTransitionFn(ctx, id, toState)
	}
	return nil
}
func (m *mockTaskTaskRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.Task])) (string, error) {
	return "", nil
}

func TestTaskList_Delegates(t *testing.T) {
	want := []domain.Task{{ID: "t1"}, {ID: "t2"}}
	taskRepo := &mockTaskTaskRepo{
		listFn: func(_ context.Context, repositoryID string) ([]domain.Task, error) {
			return want, nil
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	got, err := uc.List(context.Background(), "r1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestTaskCreate_InitialStateFromFlow(t *testing.T) {
	var capturedInitialState string
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{ID: id, Path: "/tmp/repo"}, nil
		},
	}
	taskRepo := &mockTaskTaskRepo{
		createFn: func(_ context.Context, _, _, _, initialState, _, _, worktreePath string) (domain.Task, error) {
			capturedInitialState = initialState
			return domain.Task{}, nil
		},
	}
	uc := NewTask(taskRepo, repoRepo, builtinLoader())
	_, err := uc.Create(context.Background(), "r1", "task", "feat/y", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "planning", capturedInitialState)
}

func TestTaskCreate_InvalidFlow(t *testing.T) {
	repoRepo := &mockTaskRepositoryRepo{
		getFn: func(_ context.Context, id string) (domain.Repository, error) {
			return domain.Repository{ID: id, Path: "/tmp/repo"}, nil
		},
	}
	boom := errors.New("bad flow")
	loader := &mockTaskFlowLoader{
		loadFn: func(_ string) (flow.FlowDefinition, error) {
			return flow.FlowDefinition{}, boom
		},
	}
	uc := NewTask(&mockTaskTaskRepo{}, repoRepo, loader)
	_, err := uc.Create(context.Background(), "r1", "task", "b", "main", "custom-flow.yaml")
	assert.ErrorIs(t, err, boom)
}

func TestTaskForceTransition_InvalidState(t *testing.T) {
	uc := NewTask(&mockTaskTaskRepo{}, &mockTaskRepositoryRepo{}, builtinLoader())
	err := uc.ForceTransition(context.Background(), "t1", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestTaskArchive_PropagatesError(t *testing.T) {
	boom := errors.New("archive error")
	taskRepo := &mockTaskTaskRepo{
		archiveFn: func(_ context.Context, id string) error {
			return boom
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	assert.ErrorIs(t, uc.Archive(context.Background(), "t1"), boom)
}

func TestTaskPause_PropagatesError(t *testing.T) {
	boom := errors.New("pause error")
	taskRepo := &mockTaskTaskRepo{
		pauseFn: func(_ context.Context, id string) error {
			return boom
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	assert.ErrorIs(t, uc.Pause(context.Background(), "t1"), boom)
}

func TestTaskResume_PropagatesError(t *testing.T) {
	boom := errors.New("resume error")
	taskRepo := &mockTaskTaskRepo{
		resumeFn: func(_ context.Context, id string) error {
			return boom
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	assert.ErrorIs(t, uc.Resume(context.Background(), "t1"), boom)
}

func TestTaskRetry_PropagatesError(t *testing.T) {
	boom := errors.New("retry error")
	taskRepo := &mockTaskTaskRepo{
		retryFn: func(_ context.Context, id string) error {
			return boom
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	assert.ErrorIs(t, uc.Retry(context.Background(), "t1"), boom)
}

func TestTaskForceTransition_PropagatesError(t *testing.T) {
	boom := errors.New("transition error")
	taskRepo := &mockTaskTaskRepo{
		forceTransitionFn: func(_ context.Context, id, state string) error {
			return boom
		},
	}
	uc := NewTask(taskRepo, &mockTaskRepositoryRepo{}, builtinLoader())
	assert.ErrorIs(t, uc.ForceTransition(context.Background(), "t1", "planning"), boom)
}

type mockTaskFlowLoader struct {
	loadFn func(flowPath string) (flow.FlowDefinition, error)
}

func (m *mockTaskFlowLoader) Load(flowPath string) (flow.FlowDefinition, error) {
	if m.loadFn != nil {
		return m.loadFn(flowPath)
	}
	return flow.FlowDefinition{}, nil
}
