package mocks

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProjectUsecaseMock is a test double for usecases.ProjectUsecase.
type ProjectUsecaseMock struct {
	CreateFn func(
		ctx context.Context,
		name string,
	) (domain.Project, error)

	GetFn func(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	ListFn func(
		ctx context.Context,
	) ([]domain.Project, error)

	DeleteFn func(
		ctx context.Context,
		id string,
	) error
}

func (m *ProjectUsecaseMock) Create(
	ctx context.Context,
	name string,
) (domain.Project, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, name)
	}
	return domain.Project{}, nil
}

func (m *ProjectUsecaseMock) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return domain.Project{}, nil
}

func (m *ProjectUsecaseMock) List(
	ctx context.Context,
) ([]domain.Project, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx)
	}
	return nil, nil
}

func (m *ProjectUsecaseMock) Delete(
	ctx context.Context,
	id string,
) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

// RepositoryUsecaseMock is a test double for usecases.RepositoryUsecase.
type RepositoryUsecaseMock struct {
	CreateFn func(
		ctx context.Context,
		projectID string,
		name string,
		path string,
	) (domain.Repository, error)

	GetFn func(
		ctx context.Context,
		id string,
	) (domain.Repository, error)

	ListFn func(
		ctx context.Context,
		projectID string,
	) ([]domain.Repository, error)

	DeleteFn func(
		ctx context.Context,
		id string,
	) error
}

func (m *RepositoryUsecaseMock) Create(
	ctx context.Context,
	projectID string,
	name string,
	path string,
) (domain.Repository, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, projectID, name, path)
	}
	return domain.Repository{}, nil
}

func (m *RepositoryUsecaseMock) Get(
	ctx context.Context,
	id string,
) (domain.Repository, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return domain.Repository{}, nil
}

func (m *RepositoryUsecaseMock) List(
	ctx context.Context,
	projectID string,
) ([]domain.Repository, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID)
	}
	return nil, nil
}

func (m *RepositoryUsecaseMock) Delete(
	ctx context.Context,
	id string,
) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

// TaskUsecaseMock is a test double for usecases.TaskUsecase.
type TaskUsecaseMock struct {
	CreateFn func(
		ctx context.Context,
		repositoryID string,
		title string,
		branchName string,
		baseBranch string,
		flowPath string,
	) (domain.Task, error)

	GetFn func(
		ctx context.Context,
		id string,
	) (domain.Task, error)

	ListFn func(
		ctx context.Context,
		repositoryID string,
	) ([]domain.Task, error)

	ArchiveFn func(
		ctx context.Context,
		id string,
	) error

	PauseFn func(
		ctx context.Context,
		id string,
	) error

	ResumeFn func(
		ctx context.Context,
		id string,
	) error

	RetryFn func(
		ctx context.Context,
		id string,
	) error

	ForceTransitionFn func(
		ctx context.Context,
		id string,
		state string,
	) error
}

func (m *TaskUsecaseMock) Create(
	ctx context.Context,
	repositoryID string,
	title string,
	branchName string,
	baseBranch string,
	flowPath string,
) (domain.Task, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, repositoryID, title, branchName, baseBranch, flowPath)
	}
	return domain.Task{}, nil
}

func (m *TaskUsecaseMock) Get(
	ctx context.Context,
	id string,
) (domain.Task, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return domain.Task{}, nil
}

func (m *TaskUsecaseMock) List(
	ctx context.Context,
	repositoryID string,
) ([]domain.Task, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, repositoryID)
	}
	return nil, nil
}

func (m *TaskUsecaseMock) Archive(
	ctx context.Context,
	id string,
) error {
	if m.ArchiveFn != nil {
		return m.ArchiveFn(ctx, id)
	}
	return nil
}

func (m *TaskUsecaseMock) Pause(
	ctx context.Context,
	id string,
) error {
	if m.PauseFn != nil {
		return m.PauseFn(ctx, id)
	}
	return nil
}

func (m *TaskUsecaseMock) Resume(
	ctx context.Context,
	id string,
) error {
	if m.ResumeFn != nil {
		return m.ResumeFn(ctx, id)
	}
	return nil
}

func (m *TaskUsecaseMock) Retry(
	ctx context.Context,
	id string,
) error {
	if m.RetryFn != nil {
		return m.RetryFn(ctx, id)
	}
	return nil
}

func (m *TaskUsecaseMock) ForceTransition(
	ctx context.Context,
	id string,
	state string,
) error {
	if m.ForceTransitionFn != nil {
		return m.ForceTransitionFn(ctx, id, state)
	}
	return nil
}

// AgentRunUsecaseMock is a test double for usecases.AgentRunUsecase.
type AgentRunUsecaseMock struct {
	ListFn func(
		ctx context.Context,
		taskID string,
	) ([]domain.AgentRun, error)

	InterruptFn func(
		ctx context.Context,
		id string,
	) error
}

func (m *AgentRunUsecaseMock) List(
	ctx context.Context,
	taskID string,
) ([]domain.AgentRun, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, taskID)
	}
	return nil, nil
}

func (m *AgentRunUsecaseMock) Interrupt(
	ctx context.Context,
	id string,
) error {
	if m.InterruptFn != nil {
		return m.InterruptFn(ctx, id)
	}
	return nil
}

// KanbanItemUsecaseMock is a test double for usecases.KanbanItemUsecase.
type KanbanItemUsecaseMock struct {
	ListFn func(
		ctx context.Context,
		taskID string,
	) ([]domain.KanbanItem, error)

	CreateFn func(
		ctx context.Context,
		taskID string,
		title string,
	) (domain.KanbanItem, error)

	UpdateStatusFn func(
		ctx context.Context,
		id string,
		status string,
	) error
}

func (m *KanbanItemUsecaseMock) List(
	ctx context.Context,
	taskID string,
) ([]domain.KanbanItem, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, taskID)
	}
	return nil, nil
}

func (m *KanbanItemUsecaseMock) Create(
	ctx context.Context,
	taskID string,
	title string,
) (domain.KanbanItem, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, taskID, title)
	}
	return domain.KanbanItem{}, nil
}

func (m *KanbanItemUsecaseMock) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, id, status)
	}
	return nil
}

// ReviewThreadUsecaseMock is a test double for usecases.ReviewThreadUsecase.
type ReviewThreadUsecaseMock struct {
	GetFn func(
		ctx context.Context,
		threadID string,
	) (domain.ReviewThread, error)

	ListFn func(
		ctx context.Context,
		taskID string,
		status string,
		phase string,
	) ([]domain.ReviewThread, error)

	OpenFn func(
		ctx context.Context,
		taskID string,
		file string,
		line int,
		content string,
	) (domain.ReviewThread, error)

	PostMessageFn func(
		ctx context.Context,
		threadID string,
		role string,
		content string,
	) error

	ForceApproveFn func(
		ctx context.Context,
		threadID string,
	) error
}

func (m *ReviewThreadUsecaseMock) Get(
	ctx context.Context,
	threadID string,
) (domain.ReviewThread, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, threadID)
	}
	return domain.ReviewThread{}, nil
}

func (m *ReviewThreadUsecaseMock) List(
	ctx context.Context,
	taskID string,
	status string,
	phase string,
) ([]domain.ReviewThread, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, taskID, status, phase)
	}
	return nil, nil
}

func (m *ReviewThreadUsecaseMock) Open(
	ctx context.Context,
	taskID string,
	file string,
	line int,
	content string,
) (domain.ReviewThread, error) {
	if m.OpenFn != nil {
		return m.OpenFn(ctx, taskID, file, line, content)
	}
	return domain.ReviewThread{}, nil
}

func (m *ReviewThreadUsecaseMock) PostMessage(
	ctx context.Context,
	threadID string,
	role string,
	content string,
) error {
	if m.PostMessageFn != nil {
		return m.PostMessageFn(ctx, threadID, role, content)
	}
	return nil
}

func (m *ReviewThreadUsecaseMock) ForceApprove(
	ctx context.Context,
	threadID string,
) error {
	if m.ForceApproveFn != nil {
		return m.ForceApproveFn(ctx, threadID)
	}
	return nil
}

// ConversationUsecaseMock is a test double for usecases.ConversationUsecase.
type ConversationUsecaseMock struct {
	ListFn func(
		ctx context.Context,
		taskID string,
	) ([]domain.ConversationMessage, error)
}

func (m *ConversationUsecaseMock) List(
	ctx context.Context,
	taskID string,
) ([]domain.ConversationMessage, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, taskID)
	}
	return nil, nil
}

// GitUsecaseMock is a test double for usecases.GitUsecase.
type GitUsecaseMock struct {
	LogFn func(
		ctx context.Context,
		taskID string,
	) ([]domain.GitCommit, error)

	DiffFn func(
		ctx context.Context,
		taskID string,
	) (string, error)

	ListFilesFn func(
		ctx context.Context,
		taskID string,
	) ([]string, error)
}

func (m *GitUsecaseMock) Log(
	ctx context.Context,
	taskID string,
) ([]domain.GitCommit, error) {
	if m.LogFn != nil {
		return m.LogFn(ctx, taskID)
	}
	return nil, nil
}

func (m *GitUsecaseMock) Diff(
	ctx context.Context,
	taskID string,
) (string, error) {
	if m.DiffFn != nil {
		return m.DiffFn(ctx, taskID)
	}
	return "", nil
}

func (m *GitUsecaseMock) ListFiles(
	ctx context.Context,
	taskID string,
) ([]string, error) {
	if m.ListFilesFn != nil {
		return m.ListFilesFn(ctx, taskID)
	}
	return nil, nil
}
