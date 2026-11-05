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

// Worktrees is the read port behind a chat DTO's git fields: the workspace a chat owns, its repo
// siblings, and the merge overlay resolved over them. Unwired, chats serialize without a worktree,
// which only the project-home mount does, whose rows own none.
type Worktrees interface {
	// Get returns one workspace by id.
	Get(
		ctx context.Context,
		workspaceID string,
	) (domain.Workspace, error)

	// ListInRepo returns every workspace row in a repo, with the derived Working
	// overlay applied — the sibling set eligibility is resolved over, read ONCE
	// per list rather than once per row.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)

	// MergeEligibilitiesFor resolves whether each workspace in wss can merge into
	// its local parent, in order, against siblings the caller already holds. The
	// git dry-runs are batched: one git read per parent checkout.
	MergeEligibilitiesFor(
		ctx context.Context,
		wss []domain.Workspace,
		siblings []domain.Workspace,
	) []workspace.MergeEligibility
}

// Nodes reads a workspace's Node{Kind:workspace} row, where a locked branch's sidebar placement lives.
type Nodes interface {
	GetNode(
		ctx context.Context,
		id string,
	) (tree.Node, error)
}

// worktreeScope is one request's memo of the enrichment reads, keyed per workspace since every chat of
// a workspace asks the same questions. Never held past the request: a concurrent create, delete or
// promotion would make it serve a chat a worktree it no longer holds.
type worktreeScope struct {
	handlers  *Handlers
	siblings  []domain.Workspace
	index     map[string]domain.Workspace
	owners    map[string]string
	elig      map[string]workspace.MergeEligibility
	chatRows  map[string][]domain.Chat
	placement map[string]placementAt
	// warmed, when set, closes once the background eligibility work is done;
	// nothing reads elig before then.
	warmed chan struct{}
}

func (s *worktreeScope) await() {
	if s.warmed != nil {
		<-s.warmed
	}
}

// placementAt is one workspace's resolved folder and order.
type placementAt struct {
	folderID string
	order    int
}

// repoWorktrees builds the per-row worktree closure for a chat list from one repo-wide sibling read,
// since eligibility needs siblings. A failed read yields nil (worktrees absent) rather than failing a
// list that is still worth showing; repoID "" lists the project's home workspace.
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

// chatWorkspaces is the distinct workspace ids chats name, in first-seen order.
func chatWorkspaces(chats []domain.Chat) []domain.Workspace {
	seen := map[string]bool{}
	out := []domain.Workspace{} // non-nil: nil would name the whole repo
	for _, c := range chats {
		if c.WorkspaceID == "" || seen[c.WorkspaceID] {
			continue
		}
		seen[c.WorkspaceID] = true
		out = append(out, domain.Workspace{ID: c.WorkspaceID})
	}
	return out
}

// pendingWorktrees is a repo's enrichment under way: siblings read, eligibility resolving in the
// background so the caller's other reads overlap git. The zero value enriches nothing.
type pendingWorktrees struct {
	scope *worktreeScope
}

// beginWorktrees reads the repo's siblings and starts resolving the named
// workspaces' eligibility (nil names the whole repo). Every caller must call
// done, which joins it.
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

// done joins the background eligibility work.
func (p pendingWorktrees) done() {
	if p.scope != nil {
		p.scope.await()
	}
}

// rowFn is the per-row worktree closure over the chats being listed; nil when
// nothing could be read. Those chats are every row their workspaces have, so
// the owner and placement reads answer from them instead of from the store.
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

// chatWorktree answers one chat's worktree for the by-id reads. The repo comes from the workspace, not
// the URL: by-id routes are addressed by chat alone and the home mount binds no :repoId.
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
		// An empty sibling set reads as "not mergeable", the answer a workspace with no resolvable parent
		// already gets; the row's own fields stay correct.
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

// warm resolves the merge eligibility of every named workspace the index holds
// in one batch, so a list over N worktrees costs one git read per parent
// checkout instead of N processes in series.
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

// eligibility is w's merge overlay, resolved once per scope.
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

// seedRows reuses the chats a list already read as their workspaces' rows: the repo list holds every
// conversation in the repo, and re-reading would decode every chat in the store.
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

// rowsOf is the chat rows that name workspaceID, read once per scope.
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

// project builds the workspace's own DTO and projects the chat's git half from it, so a chat and a
// workspace can never describe the same branch differently.
func (s *worktreeScope) project(
	ctx context.Context,
	c domain.Chat,
	w domain.Workspace,
) *dto.ChatWorktreeDTO {
	return dto.ChatWorktreeFrom(
		dto.WorkspaceDTOFrom(ctx, w, s.eligibility(ctx, w), s.owner(ctx, c), s.placements()))
}

// owner is the chat owning c's workspace, via domain.ResolveOwningChat (the rule workspace frames use,
// so both surfaces agree). It is not assumed to be c, which may be a thread inside the workspace.
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

// placements is the scope as a placement reader, or nil when the handlers carry
// no Node port (dto.WorkspaceDTOFrom degrades a nil reader to "" / 0).
func (s *worktreeScope) placements() dto.WorkspacePlacementReader {
	if s.handlers.nodes == nil {
		return nil
	}
	return s
}

// Placement implements dto.WorkspacePlacementReader. A fork's placement lives on its owning chat's
// ParentID/Order; only a locked branch, rendered as a branch, uses its Node{Kind:workspace} row. No
// Node yet means the repo root, first slot.
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
		// The reader answers no error; the scope caches the slot, so this logs once.
		slog.ErrorContext(ctx, "worktree: placement", "workspace", workspaceID, "err", err)
		return placementAt{}
	}
	return placementAt{n.ParentID, n.Order}
}

// OwnerOf answers the chat that owns ws: the row that records ownership, or ""
// when none does. It is a pure read — the boot reconcile gives every live
// workspace its owner (invariant D4), so a read never has to mint one.
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

// Workspaces handles GET .../repos/:repoId/workspaces: every workspace row, chat or not, since a repo's
// default checkout and never-chatted-in locked branches (and so the repo header) have no chat to derive
// from. It uses the same scope machinery as the chat path, so a workspace looks identical either way.
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

// workspacesIn is a repo's workspaces as Workspaces serves them.
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
