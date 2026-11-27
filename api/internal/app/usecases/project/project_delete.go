package project

import (
	"context"
	"fmt"
	"sort"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

type DeleteWorkspaceRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
}

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

type DeleteNodes interface {
	Forget(
		ctx context.Context,
		id string,
	) error
}

type DeleteDeps struct {
	Projects       DeleteProjectStore
	Repos          DeleteRepositoryStore
	Workspaces     DeleteWorkspaceRepo
	RepoWorkspaces DeleteRepoWorkspaces
	Nodes          DeleteNodes
	CrowbarHome    func() (string, error)
}

type DeleteUsecase interface {
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
	BeginRepoDelete(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) (domain.Repository, error)
	DeleteRepo(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	PendingDeletes(
		ctx context.Context,
	) ([]PendingDelete, error)
}

type projectDelete struct {
	deps DeleteDeps
}

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

func (u *projectDelete) teardownProject(
	ctx context.Context,
	id string,
	consent domain.DeleteConsent,
) error {
	repos, err := u.projectRepos(ctx, id)
	if err != nil {
		return err
	}
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
