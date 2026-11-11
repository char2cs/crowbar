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
	// without returning it.
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)

	// UpdateRepo applies a partial repository update in one load-mutate-save. A rename touches the
	// label only: PathSlug, the on-disk identity, is left as loaded. It answers the decided placement
	// and every Node the write shifted, so the caller announces settled state, not an async projection.
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in RepoUpdate,
	) error
	// Reorder sets a project's sidebar index and densifies the whole list, so
	// every project ends up holding a distinct 0..n-1 slot.
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)

	// Update applies a partial project update in one load-mutate-save (problem.ErrNotFound for an
	// unknown id). A rename touches the label only: Project.Path is what disk is keyed on.
	Update(
		ctx context.Context,
		projectID string,
		in Update,
	) (domain.Project, error)
	// NextHomeSlot answers the first free index at projectID's home root —
	// where a freshly imported repo's Node is minted, so the level stays
	// dense from birth instead of every new row tying at 0.
	NextHomeSlot(
		ctx context.Context,
		projectID string,
	) (int, error)
}

// Update is a partial project update: a nil field is left as it is. The icon fields are one
// three-state choice (emoji > image > default glyph); setting one without clearing the others
// leaves a project showing the icon it was told to replace.
type Update struct {
	Name          *string
	AvatarEmoji   *string
	AvatarHasIcon *bool
	// BumpAvatarVersion moves the DTO's cache-busting ?v= param. Set it whenever
	// new bytes have been written behind the stable icon URL.
	BumpAvatarVersion bool
}

// HomeWorkspaces answers which workspace is project home, so a root-level repo placement can scope
// home chats (which carry no project id) to this project. The workspace repository satisfies it:
// the workspace usecase is built after, and depends on, this package.
type HomeWorkspaces interface {
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

// Folders confirms a FolderID names a genuine project-home folder (RepoID == "") before
// validateRepoFolder writes it — the same rule checkFolderContainer enforces for chat folders.
type Folders interface {
	FindByKey(ctx context.Context, id string) (*tree.Folder, error)
}

// HomeChats answers project membership for CHAT-kind home siblings: tree.Node carries no project id,
// so only the chat aggregate can scope them. Satisfied by the chat repository (repos.AgentChat).
type HomeChats interface {
	ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error)
}

// NodePlacements is the Node surface a repo's own placement needs: minting its row (Create), the
// sibling reads a densify or placement renumbers, the SetOrder/SetPlacement writes, and Forget to
// undo an import's Create. Every home-scope sibling (chat, folder, repo) shares this one surface.
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
	// Forget purges a Node row outright, only to unwind a Create a repo import is rolling back —
	// never to delete a live row.
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

// New builds a Usecase. A nil homeChats degrades rather than fails: the bare-root densify cannot
// narrow CHAT-kind siblings to this project and includes them all.
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
		return domain.Project{}, fmt.Errorf("project: get: id %s: %w", id, problem.ErrNotFound)
	}
	return *p, nil
}

// TouchProjectActivity is a best-effort lastActivity roll-up: it looks up the
// repo's projectId, updates the project row, and logs any failure without
// returning it.
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

// Update applies a partial project update in one load-mutate-save.
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
