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

	// UpdateRepo applies a partial repository update — display name, sidebar
	// order, owning project — in one load-mutate-save, returning the updated
	// Repository so the caller can broadcast its DTO. Returns apperr.ErrNotFound
	// when no repo has the given id, or when a requested project does not exist.
	//
	// A rename touches the LABEL only. Repository.PathSlug — the repo's on-disk
	// identity, seeded once at import — is deliberately left as loaded.
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) (domain.Repository, error)
	// Reorder sets a project's sidebar index and densifies the whole list, so
	// every project ends up holding a distinct 0..n-1 slot.
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)
}

// RepoUpdate is a partial repository update: a nil field is left as it is.
// ProjectID moves the repo to another project, which also carries every
// workspace under it — see WorkspaceRelocator.
type RepoUpdate struct {
	Name      *string
	ProjectID *string
	Order     *int
}

// WorkspaceRelocator is the narrow workspace surface a repo move needs. Every
// workspace carries a denormalised ProjectID that the hierarchical routes and
// the WS namespace are keyed on, so a repo that changed projects while its
// workspaces did not would keep them and stop showing them.
type WorkspaceRelocator interface {
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	SetProject(
		ctx context.Context,
		id string,
		projectID string,
	) (domain.Workspace, error)
}

type projectUsecase struct {
	projects   store.Store[domain.Project, string]
	repos      store.ScopedStore[domain.Repository, string]
	workspaces WorkspaceRelocator
}

// New builds a Usecase from the project and repository GORM stores plus the
// workspace relocator a cross-project repo move needs. A nil relocator leaves
// the move refusing to run rather than silently stranding workspaces.
func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces WorkspaceRelocator,
) Usecase {
	return &projectUsecase{
		projects:   projects,
		repos:      repos,
		workspaces: workspaces,
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

// UpdateRepo applies a partial repository update in one load-mutate-save.
//
// A rename refreshes the generated avatar: the label + color derive from the
// name and surface only as the fallback avatar (when the repo has no custom
// icon/emoji), but must still track the name so a renamed repo's letter badge is
// correct.
//
// The row is loaded and saved whole precisely so the fields an update must NOT
// disturb travel through untouched. Repository.PathSlug is the load-bearing one:
// it is the repo's on-disk identity, seeded once at import, and every managed
// worktree already lives under it. Assigning it here would fork the repo's tree
// in two — new workspaces under the new name, the existing ones stranded under
// the old — and blind the sibling scan that rejects case-only path clashes.
//
// A project move carries the repo's workspaces with it and renumbers BOTH
// projects' repo lists. It moves nothing on disk: worktree paths were derived
// once and are stored absolute, so they keep resolving from where they are.
func (u *projectUsecase) UpdateRepo(
	ctx context.Context,
	repoID string,
	in RepoUpdate,
) (domain.Repository, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: %w", err)
	}
	if repo == nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: id %s: %w", repoID, apperr.ErrNotFound)
	}
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	origin := repo.ProjectID
	if mErr := u.applyRepoProject(ctx, repo, in.ProjectID); mErr != nil {
		return domain.Repository{}, mErr
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return domain.Repository{}, fmt.Errorf("project: update repo: save: %w", err)
	}
	if err := u.densifyRepos(ctx, repo.ProjectID, repoID, in.Order); err != nil {
		return domain.Repository{}, err
	}
	if origin != repo.ProjectID {
		if err := u.densifyRepos(ctx, origin, "", nil); err != nil {
			return domain.Repository{}, err
		}
	}
	updated, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || updated == nil {
		return *repo, nil
	}
	return *updated, nil
}

// applyRepoProject moves repo to another project, relocating every workspace
// under it. The workspace relocation runs BEFORE the repo row is saved so a
// failure leaves the repo where its workspaces still are, rather than the other
// way round.
func (u *projectUsecase) applyRepoProject(
	ctx context.Context,
	repo *domain.Repository,
	projectID *string,
) error {
	if projectID == nil || *projectID == repo.ProjectID {
		return nil
	}
	target, err := u.projects.FindByKey(ctx, *projectID)
	if err != nil {
		return fmt.Errorf("project: update repo: resolve project: %w", err)
	}
	if target == nil {
		return fmt.Errorf("project: update repo: project %s: %w", *projectID, apperr.ErrNotFound)
	}
	if u.workspaces == nil {
		return fmt.Errorf("project: update repo: no workspace relocator wired")
	}
	rows, err := u.workspaces.ListInRepo(ctx, repo.ProjectID, repo.ID)
	if err != nil {
		return fmt.Errorf("project: update repo: list workspaces: %w", err)
	}
	for _, ws := range rows {
		if _, err := u.workspaces.SetProject(ctx, ws.ID, *projectID); err != nil {
			return fmt.Errorf("project: update repo: relocate workspace %s: %w", ws.ID, err)
		}
	}
	repo.ProjectID = *projectID
	return nil
}

// densifyRepos renumbers one project's repo list 0..n-1, optionally placing
// repoID at target first, and writes back only the rows that moved.
func (u *projectUsecase) densifyRepos(
	ctx context.Context,
	projectID string,
	repoID string,
	target *int,
) error {
	rows, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("project: reorder repos: list: %w", err)
	}
	for _, moved := range place(repoIndex(rows), repoID, target) {
		rows[moved.at].Order = moved.order
		if err := u.repos.Save(ctx, rows[moved.at]); err != nil {
			return fmt.Errorf("project: reorder repos: save %s: %w", rows[moved.at].ID, err)
		}
	}
	return nil
}

// Reorder sets a project's sidebar index and densifies the whole list. Projects
// are the sidebar's top level, so the full set IS the sibling space and reading
// all of them is the scope, not an over-read.
func (u *projectUsecase) Reorder(
	ctx context.Context,
	projectID string,
	order int,
) (domain.Project, error) {
	rows, err := u.projects.FindAll(ctx)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: reorder: list: %w", err)
	}
	if !containsID(projectIndex(rows), projectID) {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, apperr.ErrNotFound)
	}
	for _, moved := range place(projectIndex(rows), projectID, &order) {
		rows[moved.at].Order = moved.order
		if err := u.projects.Save(ctx, rows[moved.at]); err != nil {
			return domain.Project{}, fmt.Errorf("project: reorder: save %s: %w", rows[moved.at].ID, err)
		}
	}
	updated, err := u.projects.FindByKey(ctx, projectID)
	if err != nil || updated == nil {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, apperr.ErrNotFound)
	}
	return *updated, nil
}
