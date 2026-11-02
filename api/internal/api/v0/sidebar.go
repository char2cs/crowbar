package v0

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// sidebar reads the projects scope's snapshot through the list routes' own
// reads, so a row the stream carries is the row REST serves.
type sidebar struct {
	app   *app.Container
	chats *chathandlers.Handlers
}

// NewSidebar is the projects scope's snapshot over c.
func NewSidebar(c *app.Container) appstream.Sidebar {
	ucs := c.Usecases
	chats := chathandlers.New(ucs.AgentChat, ucs.AgentTurn, ucs.AgentRunner, ucs.AgentAnswer,
		ucs.AgentProvider, ucs.AgentChatFolder).
		WithWorktrees(chatWorktrees{app: c}).
		WithNodes(c.Repositories.Placement)
	return &sidebar{app: c, chats: chats}
}

// Entities is every project, repo, workspace, chat and folder. A project or a
// repo whose rows will not read is left out and logged, as its list route fails
// alone; only a failed project or repo list fails the read.
func (s *sidebar) Entities(ctx context.Context) ([]appstream.Entity, error) {
	projects, err := s.app.GORM.Projects.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("v0: sidebar: projects: %w", err)
	}
	repos, err := s.app.GORM.Repositories.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("v0: sidebar: repos: %w", err)
	}
	placements := s.app.RepoPlacements.Of(ctx, repos)
	var out []appstream.Entity
	for _, p := range projects {
		out = append(out, appstream.Entity{Kind: stream.KindProject, ID: p.ID, Data: dto.ProjectDTOFrom(p)})
		out = s.appendHome(ctx, out, p.ID)
	}
	rows := s.repoRows(ctx, repos)
	for i, r := range repos {
		out = append(out, appstream.Entity{Kind: stream.KindRepo, ID: r.ID, Data: dto.RepoDTOFrom(r, placements[r.ID])})
		if rows[i] != nil {
			out = appendRows(out, *rows[i], r.ProjectID, r.ID)
		}
	}
	return out, nil
}

// repoReads is how many repos' rows are read at once: each read runs git for
// its worktrees, which the list routes did side by side, one request each.
const repoReads = 8

// repoRows reads every repo's rows side by side, in repos' order; a repo whose
// rows will not read is nil and logged.
func (s *sidebar) repoRows(ctx context.Context, repos []domain.Repository) []*chathandlers.Rows {
	out := make([]*chathandlers.Rows, len(repos))
	slots := make(chan struct{}, repoReads)
	var wg sync.WaitGroup
	for i, r := range repos {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			rows, err := s.chats.RepoRows(ctx, r.ProjectID, r.ID)
			if err != nil {
				slog.ErrorContext(ctx, "v0: sidebar: repo rows unreadable, left out", "repo", r.ID, "err", err)
				return
			}
			out[i] = &rows
		})
	}
	wg.Wait()
	return out
}

// appendHome adds projectID's home workspace and its rows.
func (s *sidebar) appendHome(ctx context.Context, out []appstream.Entity, projectID string) []appstream.Entity {
	home, err := s.app.Repositories.Workspace.GetHomeForProject(ctx, projectID)
	if err != nil {
		slog.WarnContext(ctx, "v0: sidebar: no home workspace", "project", projectID, "err", err)
		return out
	}
	rows, err := s.chats.HomeRows(ctx, home.ID)
	if err != nil {
		slog.ErrorContext(ctx, "v0: sidebar: home rows unreadable, left out", "project", projectID, "err", err)
		return out
	}
	home.Working = s.app.Repositories.WorkingFor(home.ID)
	rows.Workspaces = []dto.WorkspaceDTO{dto.WorkspaceDTOFrom(ctx, home, workspace.MergeEligibility{}, s.chats.OwnerOf(ctx, home), nil)}
	return appendRows(out, rows, projectID, "")
}

// appendRows adds one scope's rows.
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
