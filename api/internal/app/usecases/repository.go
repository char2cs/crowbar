package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RepositoryUsecase is the public contract for repository operations.
type RepositoryUsecase interface {
	Create(
		ctx context.Context,
		projectID string,
		name string,
		path string,
	) (domain.Repository, error)

	Get(
		ctx context.Context,
		id string,
	) (domain.Repository, error)

	List(
		ctx context.Context,
		projectID string,
	) ([]domain.Repository, error)

	Delete(
		ctx context.Context,
		id string,
	) error
}

type repositoryUsecase struct {
	repo        repositories.Repository
	projectRepo repositories.Project
}

// NewRepository wires a Repository repository and a Project repository into a RepositoryUsecase.
func NewRepository(
	repo repositories.Repository,
	projectRepo repositories.Project,
) RepositoryUsecase {
	return &repositoryUsecase{
		repo:        repo,
		projectRepo: projectRepo,
	}
}

func (u *repositoryUsecase) Create(
	ctx context.Context,
	projectID string,
	name string,
	path string,
) (domain.Repository, error) {
	if _, err := u.projectRepo.Get(ctx, projectID); err != nil {
		return domain.Repository{}, err
	}
	return u.repo.Create(ctx, projectID, name, path)
}

func (u *repositoryUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Repository, error) {
	return u.repo.Get(ctx, id)
}

func (u *repositoryUsecase) List(
	ctx context.Context,
	projectID string,
) ([]domain.Repository, error) {
	return u.repo.List(ctx, projectID)
}

func (u *repositoryUsecase) Delete(
	ctx context.Context,
	id string,
) error {
	return u.repo.Delete(ctx, id)
}
