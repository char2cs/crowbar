package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProjectUsecase is the public contract for project operations.
type ProjectUsecase interface {
	Create(
		ctx context.Context,
		name string,
	) (domain.Project, error)

	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	List(
		ctx context.Context,
	) ([]domain.Project, error)

	Delete(
		ctx context.Context,
		id string,
	) error
}

type projectUsecase struct {
	repo repositories.Project
}

// NewProject wires a Project repository into a ProjectUsecase.
func NewProject(
	repo repositories.Project,
) ProjectUsecase {
	return &projectUsecase{repo: repo}
}

func (u *projectUsecase) Create(
	ctx context.Context,
	name string,
) (domain.Project, error) {
	return u.repo.Create(ctx, name)
}

func (u *projectUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	return u.repo.Get(ctx, id)
}

func (u *projectUsecase) List(
	ctx context.Context,
) ([]domain.Project, error) {
	return u.repo.List(ctx)
}

func (u *projectUsecase) Delete(
	ctx context.Context,
	id string,
) error {
	return u.repo.Delete(ctx, id)
}
