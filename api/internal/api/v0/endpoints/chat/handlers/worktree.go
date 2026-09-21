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

	// MergeEligibilityFor resolves whether ws can merge into its local parent,
	// against siblings the caller already holds. It makes no repository call of
	// its own, which is why the sibling read above is hoisted out of the loop.
	MergeEligibilityFor(
		ctx context.Context,
		ws domain.Workspace,
		siblings []domain.Workspace,
	) workspace.MergeEligibility
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

// nodePlacementReader adapts Handlers.nodes/chats/worktrees to
// dto.WorkspacePlacementReader. A resolution failure (no Node row yet — see
// dto.WorkspacePlacementReader's own doc) degrades to "" / 0 rather than an
// error: this DTO is serialized for a chat list read, not a placement write,
// and a row this fix has not reached yet is honestly "at the repo root,
// first slot" until something places it.
type nodePlacementReader struct {
	nodes Nodes
	chats ChatUsecase
	wt    Worktrees
}

// Placement reads an ordinary fork's placement off its OWNING CHAT's own
// ParentID/Order — PlaceWorkspace's write for exactly this case
// (place_workspace.go's nodeID doc) lands on that same chat via
// Chats.SetPlacement/SetOrder, a real AgentChat aggregate field, never a
// Node row: no Node is ever minted for a plain chat, so reading one back via
// r.nodes here always missed, silently degrading to "" / 0 regardless of
// how long ago the drag landed. Caught live: a fork dragged into a folder
// showed it there for a moment, then reseeded straight back to the repo
// root — the response's own echoed placement was right, only the next read
// was wrong. Only a LOCKED branch, whose owning chat carries no Node of its
// own, is genuinely addressed by workspaceID's own Node{Kind:workspace} row
// — the one case r.nodes still answers.
func (r nodePlacementReader) Placement(
	ctx context.Context,
	workspaceID string,
) (folderID string, order int) {
	if ws, err := r.wt.Get(ctx, workspaceID); err == nil && !ws.RendersAsBranch() {
		if rows, cErr := r.chats.ListChatsByWorkspace(ctx, workspaceID); cErr == nil {
			if owner, ok := domain.ResolveOwningChat(rows, ws.SharedGround()); ok {
				return owner.ParentID, owner.Order
			}
		}
	}
	n, err := r.nodes.GetNode(ctx, workspaceID)
	if err != nil {
		return "", 0
	}
	return n.ParentID, n.Order
}

// placementReader answers this Handlers' own dto.WorkspacePlacementReader,
// or nil when unwired (h.nodes is nil for a test Handlers built with only
// the fields its own assertion needs, matching Worktrees' own tolerance) —
// dto.WorkspaceDTOFrom already degrades a nil reader to "" / 0.
func (h *Handlers) placementReader() dto.WorkspacePlacementReader {
	if h.nodes == nil {
		return nil
	}
	return nodePlacementReader{nodes: h.nodes, chats: h.chats, wt: h.worktrees}
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
type worktreeScope struct {
	handlers *Handlers
	siblings []domain.Workspace
	index    map[string]domain.Workspace
	owners   map[string]string
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
) func(domain.Chat) *dto.ChatWorktreeDTO {
	if h.worktrees == nil {
		return nil
	}
	siblings, err := h.worktrees.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		slog.WarnContext(ctx, "chat: list worktrees for chat enrichment",
			"project_id", projectID, "repo_id", repoID, "err", err)
		return nil
	}
	scope := h.newScope(siblings)
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
		handlers: h,
		siblings: siblings,
		index:    index,
		owners:   map[string]string{},
	}
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
	elig := s.handlers.worktrees.MergeEligibilityFor(ctx, w, s.siblings)
	return dto.ChatWorktreeFrom(
		dto.WorkspaceDTOFrom(ctx, w, elig, s.owner(ctx, c), s.handlers.placementReader()))
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
// A workspace no chat owns yet (created before the chat-first mint existed)
// gets its owner minted here (EnsureOwner) rather than answered as c's own
// id: c may be a THREAD started inside the workspace, and naming it the owner
// folded that thread into the branch row it was filed under.
func (s *worktreeScope) owner(
	ctx context.Context,
	c domain.Chat,
) string {
	if owner, ok := s.owners[c.WorkspaceID]; ok {
		return owner
	}
	owner := s.handlers.EnsureOwner(ctx, s.index[c.WorkspaceID])
	s.owners[c.WorkspaceID] = owner
	return owner
}

// EnsureOwner answers the chat that owns ws, minting one when none does.
//
// Every workspace created since the chat-first mint (MintOwningChat +
// AttachOwningWorkspace) has an owner from birth; one created before it — a
// production default checkout, an adopted locked branch, an older fork — has
// none, and every worktree verb, the files/git/terminal surfaces and the
// delete cascade are chat-keyed, so such a row was a dead end the client
// could not even address. There is no boot backfill by design; the mint
// rides the first READ instead, the same "degrade the read, mint on first
// touch" posture a Node-less repo or workspace anchor takes. Serialized
// under one lock so two concurrent lists cannot mint two owners.
//
// A legacy winner (resolved by heuristic, recording nothing) is recorded on
// the spot, so the answer is the same fact on every later read and WS frame.
//
// "" only when nothing can be minted: no tree usecase wired (tests), or the
// mint failed — the row is still served, exactly as before.
func (h *Handlers) EnsureOwner(
	ctx context.Context,
	ws domain.Workspace,
) string {
	resolve := func() (domain.Chat, bool) {
		rows, err := h.chats.ListChatsByWorkspace(ctx, ws.ID)
		if err != nil {
			return domain.Chat{}, false
		}
		return domain.ResolveOwningChat(rows, ws.SharedGround())
	}
	if owner, ok := resolve(); ok {
		h.recordOwner(ctx, owner, ws)
		return owner.ID
	}
	if ws.ID == "" || h.folders == nil || ws.Status == domain.WorkspaceStatusDeleted {
		return ""
	}
	h.ownerMint.Lock()
	defer h.ownerMint.Unlock()
	if owner, ok := resolve(); ok {
		h.recordOwner(ctx, owner, ws)
		return owner.ID
	}
	// A client abort mid-read must not tear the two-step write apart.
	ctx = context.WithoutCancel(ctx)
	chatID, err := h.folders.MintOwningChat(ctx, ws.ParentID)
	if err != nil {
		slog.WarnContext(ctx, "chat: mint the owning chat of a chatless workspace",
			"workspace_id", ws.ID, "err", err)
		return ""
	}
	if err := h.folders.AttachOwningWorkspace(ctx, chatID, ws); err != nil {
		slog.WarnContext(ctx, "chat: attach a minted owning chat to its workspace",
			"workspace_id", ws.ID, "chat_id", chatID, "err", err)
		if dErr := h.folders.DiscardOwningChat(ctx, chatID); dErr != nil {
			slog.WarnContext(ctx, "chat: discard the owning chat a failed attach left behind",
				"workspace_id", ws.ID, "chat_id", chatID, "err", dErr)
		}
		return ""
	}
	return chatID
}

// recordOwner makes a heuristically resolved legacy owner a recorded one.
func (h *Handlers) recordOwner(
	ctx context.Context,
	owner domain.Chat,
	ws domain.Workspace,
) {
	if owner.OwnsWorkspace || h.folders == nil || ws.ID == "" {
		return
	}
	if err := h.folders.AttachOwningWorkspace(ctx, owner.ID, ws); err != nil {
		slog.WarnContext(ctx, "chat: record a legacy owning chat",
			"workspace_id", ws.ID, "chat_id", owner.ID, "err", err)
	}
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
// worktreeScope.owner does: a workspace with no chats at all — the exact row
// this route exists to surface — gets its owner minted on this first read
// (EnsureOwner), so no provisioned workspace is ever served unaddressable.
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

	owners := map[string]string{}
	ownerOf := func(w domain.Workspace) string {
		if owner, ok := owners[w.ID]; ok {
			return owner
		}
		owner := h.EnsureOwner(rctx, w)
		owners[w.ID] = owner
		return owner
	}

	libs.WriteQueryOK(ctx, dto.WorkspaceDTOList(
		rctx,
		rows,
		func(w domain.Workspace) workspace.MergeEligibility {
			return h.worktrees.MergeEligibilityFor(rctx, w, rows)
		},
		ownerOf,
		h.placementReader(),
	))
}
