package project

import (
	"context"
	"fmt"
	"sort"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// DeleteProjectStore is the project persistence surface the delete usecase
// needs: resolve the project row, record its delete intent and outcome, and
// remove it.
type DeleteProjectStore interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Project, error)
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Project, error)
	Save(
		ctx context.Context,
		p domain.Project,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteRepositoryStore is the repository persistence surface the delete
// usecase needs: list every repo row (to filter by project), record a repo's
// delete intent and outcome, and remove rows.
type DeleteRepositoryStore interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
	Save(
		ctx context.Context,
		r domain.Repository,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteWorkspaceRepo is the workspace persistence surface the delete usecase
// needs: list every workspace row and tombstone the ones no repo cascade owns.
type DeleteWorkspaceRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

// DeleteRepoWorkspaces retires a repo's workspaces through the repo-delete cascade (git teardown
// that spares foreign branches, locked worktrees and, without consent, work at risk; then tombstones).
type DeleteRepoWorkspaces interface {
	DeleteRepoWorkspaces(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	RepoWorkAtRisk(
		ctx context.Context,
		repo domain.Repository,
	) ([]domain.WorkAtRisk, error)
}

// DeleteNodes drops a deleted entity's sidebar Node row.
type DeleteNodes interface {
	Forget(
		ctx context.Context,
		id string,
	) error
}

// DeleteDeps wires the delete usecase's collaborators.
type DeleteDeps struct {
	Projects       DeleteProjectStore
	Repos          DeleteRepositoryStore
	Workspaces     DeleteWorkspaceRepo
	RepoWorkspaces DeleteRepoWorkspaces
	Nodes          DeleteNodes
	CrowbarHome    func() (string, error)
}

// DeleteUsecase removes a project and everything it owns, and nothing else. Intent is recorded first
// (Deleting) so a failure is never silent and boot re-drives it; workspaces retire through the one
// lifecycle path, so the git rules hold. Without consent, work at risk is refused before any intent.
type DeleteUsecase interface {
	// BeginDelete records the delete intent and returns the row; a caller answering before the
	// teardown calls it first, so the intent is durable before the answer.
	BeginDelete(
		ctx context.Context,
		id string,
		consent domain.DeleteConsent,
	) (domain.Project, error)
	Delete(
		ctx context.Context,
		id string,
		consent domain.DeleteConsent,
	) error
	// BeginRepoDelete is BeginDelete for one repo.
	BeginRepoDelete(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) (domain.Repository, error)
	// DeleteRepo retires the repo's workspaces BEFORE its row goes, so no crash leaves workspaces
	// whose repo no longer resolves; then its row, Node row and entity directory.
	DeleteRepo(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	// PendingDeletes is every project and repo delete a crash or a failure
	// left unfinished, for boot to finish before anything is served.
	PendingDeletes(
		ctx context.Context,
	) ([]PendingDelete, error)
}

type projectDelete struct {
	deps DeleteDeps
}

// NewDelete builds a DeleteUsecase from its dependencies.
func NewDelete(
	deps DeleteDeps,
) DeleteUsecase {
	return &projectDelete{deps: deps}
}

func (u *projectDelete) BeginDelete(
	ctx context.Context,
	id string,
	consent domain.DeleteConsent,
) (domain.Project, error) {
	p, err := u.deps.Projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project delete: find project: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project delete: id %s: %w", id, problem.ErrNotFound)
	}
	if consent == domain.KeepWorkAtRisk {
		if lossErr := u.refuseProjectLoss(ctx, id); lossErr != nil {
			return domain.Project{}, lossErr
		}
	}
	p.Deleting = true
	if err := u.deps.Projects.Save(ctx, *p); err != nil {
		return domain.Project{}, fmt.Errorf("project delete: record intent: %w", err)
	}
	return *p, nil
}

func (u *projectDelete) Delete(
	ctx context.Context,
	id string,
	consent domain.DeleteConsent,
) error {
	if _, err := u.BeginDelete(ctx, id, consent); err != nil {
		return err
	}
	return u.teardownProject(ctx, id, consent)
}

// teardownProject retires everything the project owns, then its row and
// directory. Every step is idempotent, so a re-drive converges.
func (u *projectDelete) teardownProject(
	ctx context.Context,
	id string,
	consent domain.DeleteConsent,
) error {
	repos, err := u.projectRepos(ctx, id)
	if err != nil {
		return err
	}
	// Listed BEFORE any row goes: removeProjectDir needs every other project's
	// live paths, and the rows this delete tombstones are filtered out by owner.
	all, err := u.deps.Workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("project delete: list workspaces: %w", err)
	}
	for _, repo := range repos {
		if err := u.DeleteRepo(ctx, repo, consent); err != nil {
			return fmt.Errorf("project delete: %w", err)
		}
	}
	if err := u.deleteRemainingWorkspaces(ctx, id, all); err != nil {
		return err
	}
	if err := u.deps.Projects.Delete(ctx, id); err != nil {
		return fmt.Errorf("project delete: delete project %s: %w", id, err)
	}
	u.removeProjectDir(ctx, id, repos, all)
	return nil
}

func (u *projectDelete) DeleteRepo(
	ctx context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) error {
	// A caller that already recorded the intent (the HTTP handler, before its
	// 202) hands over the marked row; saving it again would change nothing.
	// The teardown itself refuses over work at risk, so no preflight here.
	if !repo.Deleting {
		var err error
		if repo, err = u.markRepoDeleting(ctx, repo); err != nil {
			return err
		}
	}
	return u.teardownRepo(ctx, repo, consent)
}

func (u *projectDelete) BeginRepoDelete(
	ctx context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) (domain.Repository, error) {
	if consent == domain.KeepWorkAtRisk {
		if err := u.refuseLoss(ctx, map[string]domain.Repository{repo.ID: repo}); err != nil {
			return domain.Repository{}, fmt.Errorf("repo %s: %w", repo.ID, err)
		}
	}
	return u.markRepoDeleting(ctx, repo)
}

func (u *projectDelete) markRepoDeleting(
	ctx context.Context,
	repo domain.Repository,
) (domain.Repository, error) {
	repo.Deleting = true
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return domain.Repository{}, fmt.Errorf("repo %s: record delete intent: %w", repo.ID, err)
	}
	return repo, nil
}

// refuseProjectLoss is refuseLoss over every repo of a project.
func (u *projectDelete) refuseProjectLoss(
	ctx context.Context,
	projectID string,
) error {
	repos, err := u.projectRepos(ctx, projectID)
	if err != nil {
		return err
	}
	if err := u.refuseLoss(ctx, repos); err != nil {
		return fmt.Errorf("project delete: %w", err)
	}
	return nil
}

// refuseLoss answers a delete without consent before it records anything: a
// domain.WorkAtRisked error naming every workspace of repos whose work it would
// destroy, or nil.
func (u *projectDelete) refuseLoss(
	ctx context.Context,
	repos map[string]domain.Repository,
) error {
	var risks []domain.WorkAtRisk
	for _, repo := range repos {
		found, err := u.deps.RepoWorkspaces.RepoWorkAtRisk(ctx, repo)
		if err != nil {
			return fmt.Errorf("repo %s: %w", repo.ID, err)
		}
		risks = append(risks, found...)
	}
	if len(risks) > 0 {
		sort.Slice(risks, func(i, j int) bool { return risks[i].Branch < risks[j].Branch })
		return domain.WorkAtRisked(risks)
	}
	return nil
}

func (u *projectDelete) teardownRepo(
	ctx context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) error {
	if err := u.deps.RepoWorkspaces.DeleteRepoWorkspaces(ctx, repo, consent); err != nil {
		return fmt.Errorf("repo %s workspaces: %w", repo.ID, err)
	}
	if err := u.deps.Repos.Delete(ctx, repo.ID); err != nil {
		return fmt.Errorf("delete repo %s: %w", repo.ID, err)
	}
	u.forgetNode(ctx, repo.ID)
	u.removeRepoDir(ctx, repo)
	return nil
}

// PendingDelete is a delete to finish: what it removes, and the work.
type PendingDelete struct {
	Kind      domain.OperationKind
	Ref       domain.EntityRef
	ProjectID string
	Label     string
	Run       func(ctx context.Context) error
}

func (u *projectDelete) PendingDeletes(
	ctx context.Context,
) ([]PendingDelete, error) {
	projects, err := u.deps.Projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("pending deletes: list projects: %w", err)
	}
	var pending []PendingDelete
	deleting := map[string]bool{}
	for _, p := range projects {
		if !p.Deleting {
			continue
		}
		deleting[p.ID] = true
		pending = append(pending, PendingDelete{
			Kind: domain.OpProjectDelete, Ref: domain.EntityRef{Kind: domain.EntityProject, ID: p.ID},
			ProjectID: p.ID, Label: "Remove " + p.Name,
			Run: func(ctx context.Context) error { return u.Delete(ctx, p.ID, domain.KeepWorkAtRisk) },
		})
	}
	repos, err := u.deps.Repos.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("pending deletes: list repos: %w", err)
	}
	for _, repo := range repos {
		// A repo of a project being deleted goes with its project.
		if !repo.Deleting || deleting[repo.ProjectID] {
			continue
		}
		pending = append(pending, PendingDelete{
			Kind: domain.OpRepoDelete, Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: repo.ID},
			ProjectID: repo.ProjectID, Label: "Remove " + repo.Name,
			Run: func(ctx context.Context) error { return u.DeleteRepo(ctx, repo, domain.KeepWorkAtRisk) },
		})
	}
	return pending, nil
}
