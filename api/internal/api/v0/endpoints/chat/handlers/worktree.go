package handlers

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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
func (h *Handlers) repoWorktrees(
	ctx context.Context,
	projectID string,
	repoID string,
) func(domain.Chat) *dto.ChatWorktreeDTO {
	if h.worktrees == nil || repoID == "" {
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
	return dto.ChatWorktreeFrom(dto.WorkspaceDTOFrom(w, elig, s.owner(ctx, c)))
}

// owner answers which chat OWNS the worktree c is describing — c itself for the
// ordinary case, and some OTHER row when c is a thread carrying its parent's
// workspace id.
//
// It reuses agentusecase.ResolveOwningChat over the workspace's own chat rows,
// which is the same call the repositories container's own owningChatIDFor makes
// as it enriches a workspace's WS frame, so the two surfaces name the same
// owner for the same worktree. Re-deriving it here with a local rule ("the
// branch-typed one", say) would be a second authority, drifting the first time
// that rule changed.
//
// An unresolvable read degrades to c's own id rather than to "": every row this
// runs for carries a workspace, so SOME chat owns it, and naming this one is
// true in the common case and never a claim about a row that does not exist —
// where "" would strip a client of the identity it needs to address the
// worktree at all.
func (s *worktreeScope) owner(
	ctx context.Context,
	c domain.Chat,
) string {
	if owner, ok := s.owners[c.WorkspaceID]; ok {
		return owner
	}
	owner := c.ID
	if rows, err := s.handlers.chats.ListChatsByWorkspace(ctx, c.WorkspaceID); err == nil {
		if resolved, found := agentusecase.ResolveOwningChat(rows); found {
			owner = resolved.ID
		}
	}
	s.owners[c.WorkspaceID] = owner
	return owner
}
