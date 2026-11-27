package v0

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type sidebar struct {
	app  *app.Container
	rows rowReader
}

type rowReader interface {
	RepoRows(ctx context.Context, projectID, repoID string) (chathandlers.Rows, error)
	HomeRows(ctx context.Context, homeID string) (chathandlers.Rows, error)
	OwnerOf(ctx context.Context, ws domain.Workspace) string
}

func NewSidebar(c *app.Container) appstream.Sidebar {
	ucs := c.Usecases
	chats := chathandlers.New(ucs.AgentChat, ucs.AgentTurn, ucs.AgentRunner, ucs.AgentAnswer,
		ucs.AgentProvider, ucs.AgentChatFolder).
		WithWorktrees(chatWorktrees{app: c}).
		WithNodes(c.Repositories.Placement)
	return &sidebar{app: c, rows: chats}
}

func (s *sidebar) Entities(ctx context.Context) (appstream.Snapshot, error) {
	ctx, end := readonce.Begin(ctx)
	defer end()
	projects, err := s.app.GORM.Projects.FindAll(ctx)
	if err != nil {
		return appstream.Snapshot{}, fmt.Errorf("v0: sidebar: projects: %w", err)
	}
	repos, err := s.app.GORM.Repositories.FindAll(ctx)
	if err != nil {
		return appstream.Snapshot{}, fmt.Errorf("v0: sidebar: repos: %w", err)
	}
	placements := s.app.RepoPlacements.Of(ctx, repos)
	var snap appstream.Snapshot
	for _, p := range projects {
		snap.Entities = append(snap.Entities, appstream.Entity{Kind: stream.KindProject, ID: p.ID, Data: dto.ProjectDTOFrom(p)})
		s.appendHome(ctx, &snap, p.ID)
	}
	rows := s.repoRows(ctx, repos)
	for i, r := range repos {
		snap.Entities = append(snap.Entities, appstream.Entity{Kind: stream.KindRepo, ID: r.ID, Data: dto.RepoDTOFrom(r, placements[r.ID])})
		if rows[i] == nil {
			snap.Unread = append(snap.Unread, r.ID)
			continue
		}
		snap.Entities = appendRows(snap.Entities, *rows[i], r.ProjectID, r.ID)
	}
	return snap, nil
}

const repoReads = 8

func (s *sidebar) repoRows(ctx context.Context, repos []domain.Repository) []*chathandlers.Rows {
	out := make([]*chathandlers.Rows, len(repos))
	slots := make(chan struct{}, repoReads)
	var wg sync.WaitGroup
	for i, r := range repos {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			rows, err := s.rows.RepoRows(ctx, r.ProjectID, r.ID)
			if err != nil {
				slog.ErrorContext(ctx, "v0: sidebar: repo rows unreadable, kept as held", "repo", r.ID, "err", err)
				return
			}
			out[i] = &rows
		})
	}
	wg.Wait()
	return out
}

func (s *sidebar) appendHome(ctx context.Context, snap *appstream.Snapshot, projectID string) {
	home, err := s.app.Repositories.Workspace.GetHomeForProject(ctx, projectID)
	if errors.Is(err, problem.ErrNotFound) {
		slog.WarnContext(ctx, "v0: sidebar: no home workspace", "project", projectID)
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "v0: sidebar: home workspace unreadable, kept as held", "project", projectID, "err", err)
		snap.Unread = append(snap.Unread, projectID)
		return
	}
	rows, err := s.rows.HomeRows(ctx, home.ID)
	if err != nil {
		slog.ErrorContext(ctx, "v0: sidebar: home rows unreadable, kept as held", "project", projectID, "err", err)
		snap.Unread = append(snap.Unread, projectID)
		return
	}
	home.Working = s.app.Repositories.WorkingFor(home.ID)
	rows.Workspaces = []dto.WorkspaceDTO{dto.WorkspaceDTOFrom(ctx, home, workspace.MergeEligibility{}, s.rows.OwnerOf(ctx, home), nil)}
	snap.Entities = appendRows(snap.Entities, rows, projectID, "")
}

func appendRows(out []appstream.Entity, rows chathandlers.Rows, projectID, repoID string) []appstream.Entity {
	for _, w := range rows.Workspaces {
		if w.Status == domain.WorkspaceStatusDeleted {
			continue
		}
		out = append(out, appstream.Entity{Kind: stream.KindWorkspace, ID: w.ID, Data: w})
	}
	for _, f := range rows.Folders {
		out = append(out, appstream.Entity{Kind: stream.KindFolder, ID: f.ID, Data: folderEntity(f, projectID, repoID)})
	}
	for _, c := range rows.Chats {
		out = append(out, appstream.Entity{Kind: stream.KindChatEntity, ID: c.ID, Data: dto.ChatEntityFrom(c, projectID, repoID)})
	}
	return out
}

func folderEntity(f dto.AgentChatDTO, projectID, repoID string) dto.FolderEntityDTO {
	return dto.FolderEntityDTO{
		ID: f.ID, ProjectID: projectID, RepoID: repoID,
		ParentID: f.ParentID, Name: f.Title, Order: f.Order, CreatedAt: f.CreatedAt,
	}
}
