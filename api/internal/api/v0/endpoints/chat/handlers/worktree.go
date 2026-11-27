package handlers

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Worktrees interface {
	Get(
		ctx context.Context,
		workspaceID string,
	) (domain.Workspace, error)

	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)

	MergeEligibilitiesFor(
		ctx context.Context,
		wss []domain.Workspace,
		siblings []domain.Workspace,
	) []workspace.MergeEligibility
}

type Nodes interface {
	GetNode(
		ctx context.Context,
		id string,
	) (tree.Node, error)
}

type worktreeScope struct {
	handlers  *Handlers
	siblings  []domain.Workspace
	index     map[string]domain.Workspace
	owners    map[string]string
	elig      map[string]workspace.MergeEligibility
	chatRows  map[string][]domain.Chat
	placement map[string]placementAt
	warmed    chan struct{}
}

func (s *worktreeScope) await() {
	if s.warmed != nil {
		<-s.warmed
	}
}

type placementAt struct {
	folderID string
	order    int
}

func (h *Handlers) repoWorktrees(
	ctx context.Context,
	projectID string,
	repoID string,
	chats []domain.Chat,
) func(domain.Chat) *dto.ChatWorktreeDTO {
	pending := h.beginWorktrees(ctx, projectID, repoID, chatWorkspaces(chats))
	defer pending.done()
	return pending.rowFn(ctx, chats)
}

func chatWorkspaces(chats []domain.Chat) []domain.Workspace {
	seen := map[string]bool{}
	out := []domain.Workspace{}
	for _, c := range chats {
		if c.WorkspaceID == "" || seen[c.WorkspaceID] {
			continue
		}
		seen[c.WorkspaceID] = true
		out = append(out, domain.Workspace{ID: c.WorkspaceID})
	}
	return out
}

type pendingWorktrees struct {
	scope *worktreeScope
}

func (h *Handlers) beginWorktrees(
	ctx context.Context,
	projectID string,
	repoID string,
	named []domain.Workspace,
) pendingWorktrees {
	if h.worktrees == nil {
		return pendingWorktrees{}
	}
	siblings, err := h.worktrees.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		slog.WarnContext(ctx, "chat: list worktrees for chat enrichment",
			"project_id", projectID, "repo_id", repoID, "err", err)
		return pendingWorktrees{}
	}
	scope := h.newScope(siblings)
	if named == nil {
		named = siblings
	}
	scope.warmed = make(chan struct{})
	go func() {
		defer close(scope.warmed)
		scope.warm(ctx, named)
	}()
	return pendingWorktrees{scope: scope}
}

func (p pendingWorktrees) done() {
	if p.scope != nil {
		p.scope.await()
	}
}

func (p pendingWorktrees) rowFn(
	ctx context.Context,
	chats []domain.Chat,
) func(domain.Chat) *dto.ChatWorktreeDTO {
	scope := p.scope
	if scope == nil {
		return nil
	}
	scope.seedRows(chats)
	return func(c domain.Chat) *dto.ChatWorktreeDTO {
		w, ok := scope.index[c.WorkspaceID]
		if c.WorkspaceID == "" || !ok {
			return nil
		}
		return scope.project(ctx, c, w)
	}
}

func (h *Handlers) chatWorktree(
	ctx context.Context,
	c domain.Chat,
) *dto.ChatWorktreeDTO {
	if h.worktrees == nil || c.WorkspaceID == "" {
		return nil
	}
	w, err := h.worktrees.Get(ctx, c.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "chat: read the worktree this chat owns",
			"chat_id", c.ID, "workspace_id", c.WorkspaceID, "err", err)
		return nil
	}
	siblings, err := h.worktrees.ListInRepo(ctx, w.ProjectID, w.RepoID)
	if err != nil {
		slog.WarnContext(ctx, "chat: list siblings for the worktree this chat owns",
			"chat_id", c.ID, "workspace_id", c.WorkspaceID, "err", err)
		siblings = nil
	}
	return h.newScope(siblings).project(ctx, c, w)
}

func (h *Handlers) newScope(
	siblings []domain.Workspace,
) *worktreeScope {
	index := make(map[string]domain.Workspace, len(siblings))
	for _, w := range siblings {
		index[w.ID] = w
	}
	return &worktreeScope{
		handlers:  h,
		siblings:  siblings,
		index:     index,
		owners:    map[string]string{},
		elig:      map[string]workspace.MergeEligibility{},
		chatRows:  map[string][]domain.Chat{},
		placement: map[string]placementAt{},
	}
}

func (s *worktreeScope) warm(
	ctx context.Context,
	named []domain.Workspace,
) {
	var todo []domain.Workspace
	for _, n := range named {
		if w, ok := s.index[n.ID]; ok {
			todo = append(todo, w)
		}
	}
	for i, e := range s.handlers.worktrees.MergeEligibilitiesFor(ctx, todo, s.siblings) {
		s.elig[todo[i].ID] = e
	}
}

func (s *worktreeScope) eligibility(
	ctx context.Context,
	w domain.Workspace,
) workspace.MergeEligibility {
	s.await()
	if e, ok := s.elig[w.ID]; ok {
		return e
	}
	e := s.handlers.worktrees.MergeEligibilitiesFor(ctx, []domain.Workspace{w}, s.siblings)[0]
	s.elig[w.ID] = e
	return e
}

func (s *worktreeScope) seedRows(chats []domain.Chat) {
	seeded := map[string][]domain.Chat{}
	for _, c := range chats {
		if c.WorkspaceID != "" {
			seeded[c.WorkspaceID] = append(seeded[c.WorkspaceID], c)
		}
	}
	for id, rows := range seeded {
		s.chatRows[id] = rows
	}
}

func (s *worktreeScope) rowsOf(
	ctx context.Context,
	workspaceID string,
) []domain.Chat {
	if rows, ok := s.chatRows[workspaceID]; ok {
		return rows
	}
	rows, err := s.handlers.chats.ListChatsByWorkspace(ctx, workspaceID)
	if err != nil {
		rows = nil
	}
	s.chatRows[workspaceID] = rows
	return rows
}

func (s *worktreeScope) project(
	ctx context.Context,
	c domain.Chat,
	w domain.Workspace,
) *dto.ChatWorktreeDTO {
	return dto.ChatWorktreeFrom(
		dto.WorkspaceDTOFrom(ctx, w, s.eligibility(ctx, w), s.owner(ctx, c), s.placements()))
}

func (s *worktreeScope) owner(
	ctx context.Context,
	c domain.Chat,
) string {
	if owner, ok := s.owners[c.WorkspaceID]; ok {
		return owner
	}
	owner, _ := domain.ResolveOwningChat(s.rowsOf(ctx, c.WorkspaceID))
	s.owners[c.WorkspaceID] = owner.ID
	return owner.ID
}

func (s *worktreeScope) placements() dto.WorkspacePlacementReader {
	if s.handlers.nodes == nil {
		return nil
	}
	return s
}

func (s *worktreeScope) Placement(
	ctx context.Context,
	workspaceID string,
) (string, int) {
	if p, ok := s.placement[workspaceID]; ok {
		return p.folderID, p.order
	}
	p := s.placementOf(ctx, workspaceID)
	s.placement[workspaceID] = p
	return p.folderID, p.order
}

func (s *worktreeScope) placementOf(
	ctx context.Context,
	workspaceID string,
) placementAt {
	if ws, ok := s.index[workspaceID]; ok && !ws.RendersAsBranch() {
		if owner, found := domain.ResolveOwningChat(s.rowsOf(ctx, workspaceID)); found {
			return placementAt{owner.ParentID, owner.Order}
		}
	}
	n, err := tree.OrUnplaced(s.handlers.nodes.GetNode(ctx, workspaceID))
	if err != nil {
		slog.ErrorContext(ctx, "worktree: placement", "workspace", workspaceID, "err", err)
		return placementAt{}
	}
	return placementAt{n.ParentID, n.Order}
}

func (h *Handlers) OwnerOf(
	ctx context.Context,
	ws domain.Workspace,
) string {
	rows, err := h.chats.ListChatsByWorkspace(ctx, ws.ID)
	if err != nil {
		return ""
	}
	owner, _ := domain.ResolveOwningChat(rows)
	return owner.ID
}

func (h *Handlers) Workspaces(
	ctx *gin.Context,
) {
	rows, err := h.workspacesIn(ctx.Request.Context(), ctx.Param("projectId"), ctx.Param("repoId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, rows)
}

func (h *Handlers) workspacesIn(ctx context.Context, projectID, repoID string) ([]dto.WorkspaceDTO, error) {
	if h.worktrees == nil {
		return []dto.WorkspaceDTO{}, nil
	}
	rows, err := h.worktrees.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("chat: workspaces of %s: %w", repoID, err)
	}
	scope := h.newScope(rows)
	scope.warm(ctx, rows)
	return dto.WorkspaceDTOList(
		ctx,
		rows,
		func(w domain.Workspace) workspace.MergeEligibility { return scope.eligibility(ctx, w) },
		func(w domain.Workspace) string {
			return scope.owner(ctx, domain.Chat{WorkspaceID: w.ID})
		},
		scope.placements(),
	), nil
}
