package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)


// TaskUsecase is the public contract for task operations.
type TaskUsecase interface {
	Create(
		ctx context.Context,
		repositoryID string,
		title string,
		branchName string,
		baseBranch string,
		flowPath string,
	) (domain.Task, error)

	Get(
		ctx context.Context,
		id string,
	) (domain.Task, error)

	List(
		ctx context.Context,
		repositoryID string,
	) ([]domain.Task, error)

	Archive(
		ctx context.Context,
		id string,
	) error

	Pause(
		ctx context.Context,
		id string,
	) error

	Resume(
		ctx context.Context,
		id string,
	) error

	Retry(
		ctx context.Context,
		id string,
	) error

	ForceTransition(
		ctx context.Context,
		id string,
		state string,
	) error
}

type taskUsecase struct {
	repo       repositories.Task
	repoRepo   repositories.Repository
	flowLoader flow.Loader
}

// NewTask wires a Task repository, a Repository repository, and a flow.Loader into a TaskUsecase.
func NewTask(
	repo repositories.Task,
	repoRepo repositories.Repository,
	flowLoader flow.Loader,
) TaskUsecase {
	return &taskUsecase{
		repo:       repo,
		repoRepo:   repoRepo,
		flowLoader: flowLoader,
	}
}

func (u *taskUsecase) Create(
	ctx context.Context,
	repositoryID string,
	title string,
	branchName string,
	baseBranch string,
	flowPath string,
) (domain.Task, error) {
	repo, err := u.repoRepo.Get(ctx, repositoryID)
	if err != nil {
		return domain.Task{}, err
	}
	flowDef, err := u.flowLoader.Load(flowPath)
	if err != nil {
		return domain.Task{}, fmt.Errorf("task: invalid flow: %w", errors.Join(domain.ErrBadRequest, err))
	}
	if len(flowDef.States) == 0 {
		return domain.Task{}, fmt.Errorf("task: flow has no states")
	}
	initialState := flowDef.States[0].Name
	worktreePath := repo.Path + "/.worktrees/" + branchName
	return u.repo.Create(ctx, repositoryID, title, flowPath, initialState, branchName, baseBranch, repo.Path, worktreePath)
}

func (u *taskUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Task, error) {
	return u.repo.Get(ctx, id)
}

func (u *taskUsecase) List(
	ctx context.Context,
	repositoryID string,
) ([]domain.Task, error) {
	return u.repo.List(ctx, repositoryID)
}

func (u *taskUsecase) Archive(
	ctx context.Context,
	id string,
) error {
	return u.repo.Archive(ctx, id)
}

func (u *taskUsecase) Pause(
	ctx context.Context,
	id string,
) error {
	return u.repo.Pause(ctx, id)
}

func (u *taskUsecase) Resume(
	ctx context.Context,
	id string,
) error {
	return u.repo.Resume(ctx, id)
}

func (u *taskUsecase) Retry(
	ctx context.Context,
	id string,
) error {
	return u.repo.Retry(ctx, id)
}

func (u *taskUsecase) ForceTransition(
	ctx context.Context,
	id string,
	state string,
) error {
	task, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	flowDef, err := u.flowLoader.Load(task.FlowPath)
	if err != nil {
		return fmt.Errorf("task: load flow: %w", err)
	}
	for _, s := range flowDef.States {
		if s.Name == state {
			return u.repo.ForceTransition(ctx, id, state)
		}
	}
	return fmt.Errorf("task: state %q not found in flow: %w", state, domain.ErrBadRequest)
}
