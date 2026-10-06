package handlers

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Worktrees is the narrow read port the chat DTO's worktree enrichment needs
// (spec §5, "the Chat DTO gains the git fields Workspace's own DTO currently
// carries"): the workspace a chat owns, that workspace's repo siblings, and the
// merge-eligibility overlay resolved over them.
//
// Declared here, by the consumer (law 4), and satisfied in the container with
// whatever concrete types actually implement it (law 6). The three verbs are on
// ONE port because they are one question from here — "what is this chat's
// worktree, as a client should see it" — and answering it needs all three: the
// row, the siblings eligibility is resolved against, and the resolver itself.
//
// Left unwired, every chat serializes without a worktree. That is a legible
// degradation rather than a silent one: the surfaces that mount these handlers
// without it (the project-home group, whose row has no repo and no git surface
// at all) serve rows that own no worktree anyway.
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

// Nodes is the narrow read port a worktree-owning chat's DTO needs to carry
// its own sidebar placement (2026-09-09 sidebar-placement-unification,
// workspace-placement fix): the workspace's own Node{Kind:workspace} row —
// the SAME position PlaceWorkspace writes — read back so the panel a drag
// just wrote to actually redraws it.
type Nodes interface {
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
}

// worktreeScope is ONE read's worth of the answers the enrichment needs: the
// repo's workspace rows, and the owning chat resolved per workspace.
//
// It is built per call and thrown away, never held on Handlers. That is
// deliberate rather than wasteful: both halves are snapshots of state a
// concurrent create, delete or promotion moves, and a memo that outlived the
// request would serve a chat the branch of a worktree it no longer holds — the
// exact class of staleness this whole refactor exists to delete. Within one
// response it is pure win, since a list of twenty chats sharing four worktrees
// takes four reads instead of twenty.
//
// Everything it answers is memoized per WORKSPACE, because every chat of one
// workspace asks the same questions: eligibility (a git dry-run), the chat rows
// that name the owner and the placement (store reads). The distinct
// workspaces' dry-runs are batched by warm.
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

// repoWorktrees builds the per-row worktree closure a repo-scoped chat list is
// serialized through: one ListInRepo for the whole list, indexed by id, and
// every row resolved out of that index.
//
// It is the direct counterpart of the retired workspace list's own snapshot,
// which composed its eligibility over one repo-wide read for exactly the same
// reason: resolving eligibility needs the row's siblings, and reading them per
// row would turn one list into one repo-wide read per chat.
//
// A read that fails yields a nil closure — every row's worktree absent — rather
// than an error, and the same is true of a chat naming a workspace the index
// does not hold. That degradation is deliberate and matches resolveOwningChatID's
// own: the caller asked for the chat list, and a chat whose git state cannot be
// resolved is still a chat worth listing. The alternative — failing the whole
// list — would blank the panel over one unreadable row.
//
// The home mount binds no :repoId, and repoID "" lists exactly the project's
// home workspace (it rides no repo), so the home owner's row carries
// worktree.owningChatId == its own id the same way a repo's does. Without
// that the client had no marker to keep the owner off the tree, and it drew
// as an "Untitled chat" ghost row above every real home chat.
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

// pendingWorktrees is a repo's worktree enrichment already under way: the
// sibling read is done and every workspace's merge eligibility is resolving in
// the background, so a caller with other reads to make overlaps them with git.
// The zero value, from unwired or failed reads, enriches nothing.
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

// chatWorktree answers ONE chat's worktree, for the by-id reads (Get, Promote,
// a placement's echoed row) that have no list to amortise a repo-wide read
// over.
//
// It resolves the workspace first and takes its OWN repo from the row rather
// than from the URL, because the by-id routes are addressed by chat alone: the
// home mount binds no :repoId, and a chat's repo is derived from the workspace
// it lands on, never asserted by the caller.
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
		// The row itself is readable and its own fields are already correct; only
		// the merge overlay is not. Serializing it against an empty sibling set
		// says "not mergeable", which is the same answer a workspace with no
		// resolvable parent already gets — never a wrong branch name.
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

// seedRows takes the chats a list already read as the rows of the workspaces
// they name. A workspace's own chats are all in that list — the repo's list
// holds every conversation whose workspace is in the repo — so reading them
// again, which decodes every chat in the store, would only repeat it.
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

// project is the one place a worktree becomes wire bytes, for both the list
// path and the by-id path: it builds the workspace's OWN DTO and projects the
// git half out of it, so a chat and a workspace can never describe the same
// branch differently (see dto.ChatWorktreeFrom).
func (s *worktreeScope) project(
	ctx context.Context,
	c domain.Chat,
	w domain.Workspace,
) *dto.ChatWorktreeDTO {
	return dto.ChatWorktreeFrom(
		dto.WorkspaceDTOFrom(ctx, w, s.eligibility(ctx, w), s.owner(ctx, c), s.placements()))
}

// owner answers which chat OWNS the worktree c is describing — c itself for the
// ordinary case, and some OTHER row when c is a thread carrying its parent's
// workspace id.
//
// It reuses domain.ResolveOwningChat over the workspace's own chat rows,
// which is the same call the repositories container's own owningChatIDFor makes
// as it enriches a workspace's WS frame, so the two surfaces name the same
// owner for the same worktree. Re-deriving it here with a local rule ("the
// branch-typed one", say) would be a second authority, drifting the first time
// that rule changed.
//
// It is never answered as c's own id: c may be a THREAD started inside the
// workspace, and naming it the owner folded that thread into the branch row it
// was filed under.
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

// Placement implements dto.WorkspacePlacementReader over the scope's own reads.
//
// An ordinary fork's placement lives on its OWNING CHAT's own ParentID/Order —
// PlaceWorkspace writes it there through Chats.SetPlacement/SetOrder, and no
// Node is ever minted for a plain chat, so a Node lookup always missed and
// snapped a dragged fork back to the repo root. Only a LOCKED branch, whose
// owning chat carries no Node of its own, is addressed by the workspace's own
// Node{Kind:workspace} row. A row with no Node yet degrades to "" / 0: the repo
// root, first slot, until something places it.
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
	n, err := s.handlers.nodes.GetNode(ctx, workspaceID)
	if err != nil {
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

// Workspaces handles GET .../repos/:repoId/workspaces: every workspace row in
// the repo, chat or no chat.
//
// This is the resource fetchWorkspaces's own doc (web/src/lib/api.ts) says
// does not exist any more — workspaces have been derived from the chat list
// since the chat-scoped API redesign, on the assumption that every worktree
// worth showing has a chat to derive it from. That assumption breaks for a
// repo's own default/main checkout and for any locked tracking branch nobody
// has ever chatted in: no chat, no derived DTO, no row — not just missing
// content but a missing REPO HEADER, since rows-from-repo.ts mints that from
// the default workspace. Caught live: an entire repo (and a repo's own other
// locked branches) silently absent from the sidebar the instant it had no
// chats, reproduced against a real production data set.
//
// Reuses the exact same worktreeScope/WorkspaceDTOFrom machinery
// repoWorktrees already built for the chat-derived path — same eligibility
// resolution, same placement overlay, same DTO shape — so a workspace looks
// identical whether the client learns of it through a chat or through this
// route directly. ownerOf resolves each row's owning chat id the same way
// worktreeScope.owner does (OwnerOf).
func (h *Handlers) Workspaces(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	projectID, repoID := ctx.Param("projectId"), ctx.Param("repoId")

	if h.worktrees == nil {
		libs.WriteQueryOK(ctx, []dto.WorkspaceDTO{})
		return
	}
	rows, err := h.worktrees.ListInRepo(rctx, projectID, repoID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	scope := h.newScope(rows)
	scope.warm(rctx, rows)

	libs.WriteQueryOK(ctx, dto.WorkspaceDTOList(
		rctx,
		rows,
		func(w domain.Workspace) workspace.MergeEligibility { return scope.eligibility(rctx, w) },
		func(w domain.Workspace) string {
			return scope.owner(rctx, domain.Chat{WorkspaceID: w.ID})
		},
		scope.placements(),
	))
}
