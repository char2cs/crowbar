package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	store "github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Usecase interface {
	List(
		ctx context.Context,
	) ([]domain.Project, error)

	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)

	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)

	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) error
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)

	Update(
		ctx context.Context,
		projectID string,
		in Update,
	) (domain.Project, error)
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

type Update struct {
	Name              *string
	AvatarEmoji       *string
	AvatarHasIcon     *bool
	BumpAvatarVersion bool
}

type HomeWorkspaces interface {
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

type Folders interface {
	FindByKey(ctx context.Context, id string) (*tree.Folder, error)
}

type HomeChats interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
}

type NodePlacements interface {
	Create(
		ctx context.Context,
		id string,
		kind tree.Kind,
		parentID string,
		order int,
	) (tree.Node, error)
	GetNode(
		ctx context.Context,
		id string,
	) (tree.Node, error)
	ListByParent(
		ctx context.Context,
		parentID string,
	) ([]tree.Node, error)
	SetOrder(
		ctx context.Context,
		id string,
		order int,
	) error
	SetPlacement(
		ctx context.Context,
		id string,
		parentID string,
		order int,
	) error
	Forget(
		ctx context.Context,
		id string,
	) error
}

type projectUsecase struct {
	projects   store.Store[domain.Project, string]
	repos      store.ScopedStore[domain.Repository, string]
	workspaces HomeWorkspaces
	folders    Folders
	nodes      NodePlacements
	homeChats  HomeChats
}

func New(
	projects store.Store[domain.Project, string],
	repos store.ScopedStore[domain.Repository, string],
	workspaces HomeWorkspaces,
	folders Folders,
	nodes NodePlacements,
	homeChats HomeChats,
) Usecase {
	return &projectUsecase{
		projects:   projects,
		repos:      repos,
		workspaces: workspaces,
		folders:    folders,
		nodes:      nodes,
		homeChats:  homeChats,
	}
}

func (u *projectUsecase) List(
	ctx context.Context,
) ([]domain.Project, error) {
	list, err := u.projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	return list, nil
}

func (u *projectUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Project, error) {
	p, err := u.projects.FindByKey(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: get: %w", err)
	}
	if p == nil {
		return domain.Project{}, fmt.Errorf("project: get: id %s: %w", id, problem.ErrNotFound)
	}
	return *p, nil
}

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
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, problem.ErrNotFound)
	}
	for _, moved := range place(projectIndex(rows), projectID, &order) {
		rows[moved.at].Order = moved.order
		if err := u.projects.Save(ctx, rows[moved.at]); err != nil {
			return domain.Project{}, fmt.Errorf("project: reorder: save %s: %w", rows[moved.at].ID, err)
		}
	}
	updated, err := u.projects.FindByKey(ctx, projectID)
	if err != nil || updated == nil {
		return domain.Project{}, fmt.Errorf("project: reorder: id %s: %w", projectID, problem.ErrNotFound)
	}
	return *updated, nil
}

func (u *projectUsecase) Update(
	ctx context.Context,
	projectID string,
	in Update,
) (domain.Project, error) {
	row, err := u.projects.FindByKey(ctx, projectID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("project: update: load %s: %w", projectID, err)
	}
	if row == nil {
		return domain.Project{}, fmt.Errorf("project: update: id %s: %w", projectID, problem.ErrNotFound)
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return domain.Project{}, fmt.Errorf("project: update: name is empty: %w", problem.ErrInvalidArgument)
		}
		row.Name = name
	}
	if in.AvatarEmoji != nil {
		row.AvatarEmoji = *in.AvatarEmoji
	}
	if in.AvatarHasIcon != nil {
		row.AvatarHasIcon = *in.AvatarHasIcon
	}
	if in.BumpAvatarVersion {
		row.AvatarVersion++
	}
	if err := u.projects.Save(ctx, *row); err != nil {
		return domain.Project{}, fmt.Errorf("project: update: save %s: %w", projectID, err)
	}
	return *row, nil
}
