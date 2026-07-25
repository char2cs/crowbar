package project

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Usecase is the read + roll-up surface for the Project entity.
type Usecase interface {
	// List returns every project stored in the GORM table.
	List(
		ctx context.Context,
	) ([]domain.Project, error)

	// Get returns the project with the given id, or an error when not found.
	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up
	// the repo's projectId, updates the project row, and logs any failure
	// without returning it (00 §5.1).
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)

	// RenameRepo sets a repository's display name and refreshes its generated
	// avatar (label + color derive from the name), returning the updated
	// Repository so the caller can broadcast its DTO. Returns apperr.ErrNotFound
	// when no repo has the given id.
	//
	// It touches the LABEL only. Repository.PathSlug — the repo's on-disk
	// identity, seeded once at import — is deliberately left as loaded.
	RenameRepo(
		ctx context.Context,
		repoID string,
		name string,
	) (domain.Repository, error)
}

type projectUsecase struct {
	projects store.Store[domain.Project, string]
	repos    store.Store[domain.Repository, string]
}

// New builds a Usecase from the project and repository GORM stores.
func New(
	projects store.Store[domain.Project, string],
	repos store.Store[domain.Repository, string],
) Usecase {
	return &projectUsecase{
		projects: projects,
		repos:    repos,
	}
}

// List returns every project stored in the GORM table.
func (u *projectUsecase) List(
	ctx context.Context,
) ([]domain.Project, error) {
	list, err := u.projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	return list, nil
}

// Get returns the project with the given id, or an error when not found.
func (u *projectUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	p, err := u.projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: get: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project: get: id %s: %w", id, apperr.ErrNotFound)
	}
	return *p, nil
}

// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up the
// repo's projectId, updates the project row, and logs any failure without
// returning it (00 §5.1).
func (u *projectUsecase) TouchProjectActivity(
	ctx context.Context,
	repoID string,
	now time.Time,
) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		slog.WarnContext(ctx, "project: touch activity: repo not found", "repoID", repoID)
		return
	}
	project, err := u.projects.FindByKey(ctx, repo.ProjectID)
	if err != nil || project == nil {
		slog.WarnContext(ctx, "project: touch activity: project not found", "projectID", repo.ProjectID)
		return
	}
	project.LastActivity = now
	if err := u.projects.Save(ctx, *project); err != nil {
		slog.ErrorContext(ctx, "project: touch activity: save failed", "projectID", project.ID, "err", err)
	}
}

// RenameRepo sets a repository's display name and refreshes its generated
// avatar. The label + color are derived from the name and surface only as the
// fallback avatar (when the repo has no custom icon/emoji), but must still track
// the name so a renamed repo's letter badge is correct. Returns the updated
// Repository, or apperr.ErrNotFound when the id is unknown.
//
// The row is loaded and saved whole precisely so the fields a rename must NOT
// disturb travel through untouched. Repository.PathSlug is the load-bearing one:
// it is the repo's on-disk identity, seeded once at import, and every managed
// worktree already lives under it. Assigning it here would fork the repo's tree
// in two — new workspaces under the new name, the existing ones stranded under
// the old — and blind the sibling scan that rejects case-only path clashes.
func (u *projectUsecase) RenameRepo(
	ctx context.Context,
	repoID string,
	name string,
) (domain.Repository, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("project: rename repo: %w", err)
	}
	if repo == nil {
		return domain.Repository{}, fmt.Errorf("project: rename repo: id %s: %w", repoID, apperr.ErrNotFound)
	}
	repo.Name = name
	repo.AvatarLabel = avatar.Label(name)
	repo.AvatarColor = avatar.Color(name)
	if err := u.repos.Save(ctx, *repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project: rename repo: save: %w", err)
	}
	return *repo, nil
}
