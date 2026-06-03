package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// KanbanItemUsecase is the public contract for kanban item operations.
type KanbanItemUsecase interface {
	List(
		ctx context.Context,
		taskID string,
	) ([]domain.KanbanItem, error)

	Create(
		ctx context.Context,
		taskID string,
		title string,
	) (domain.KanbanItem, error)

	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
	) error
}

type kanbanItemUsecase struct {
	repo repositories.KanbanItem
}

// NewKanbanItem wires a KanbanItem repository into a KanbanItemUsecase.
func NewKanbanItem(
	repo repositories.KanbanItem,
) KanbanItemUsecase {
	return &kanbanItemUsecase{repo: repo}
}

func (u *kanbanItemUsecase) List(
	ctx context.Context,
	taskID string,
) ([]domain.KanbanItem, error) {
	return u.repo.ListByTask(ctx, taskID)
}

func (u *kanbanItemUsecase) Create(
	ctx context.Context,
	taskID string,
	title string,
) (domain.KanbanItem, error) {
	return u.repo.Create(ctx, taskID, title, "")
}

func (u *kanbanItemUsecase) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	return u.repo.UpdateStatus(ctx, id, status, "")
}
