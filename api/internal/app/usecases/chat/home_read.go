package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Fix round 2 of 2026-09-08 sidebar-placement-unification Task 5: a
// home-scoped chat's Chat.ParentID/.Order are write-once-at-creation-then-
// ignored (position authority moved to Node — see tree/internal's
// correctHomePlacement, which fixes this for the TREE package's own
// PLANNING reads). The actual read endpoints (GET /chats, GET /chats/:id)
// never went through that path at all — ChatUsecase.ListChatsByWorkspace/
// ListChatsInRepo/ListChats/GetChat read straight off the Chat aggregate's
// own frozen fields, through Conversations, with zero Node awareness. A chat
// moved into a home folder therefore rendered at the panel root FOREVER,
// survivably a reload, since the reload hits this exact same stale path.
//
// NewHomeCorrectedChats wraps a ChatUsecase so every row it returns carries
// its LIVE position for a home-scoped chat, overlaying the Node-sourced
// ParentID/Order the same way correctHomePlacement does — kept as an
// independent, small implementation rather than exported from tree/internal:
// this decorator's lifecycle (wraps the read surface every handler shares)
// is unrelated to a placement plan's, and the logic itself is five lines.
//
// This does NOT close the companion gap the SDD review also flagged: a WS
// broadcast for a chat's move (unlike a rename or a delete) only ever fires
// announceFolders, filtered to folder-kind rows — a live viewer's already-
// open session does not see the move until it re-fetches. Deliberately
// deferred: fixing it means teaching the hub's chat-lifecycle broadcast
// about Node moves, a materially larger change than this read-side fix, and
// Task 6 (frontend home-tree work) is positioned to decide whether the
// reload-only guarantee is acceptable for now or needs a live push too.
func NewHomeCorrectedChats(
	chats ChatUsecase,
	workspaces TreeWorkspaceGitStatus,
	nodes TreeNodes,
) ChatUsecase {
	return &homeCorrectedChats{ChatUsecase: chats, workspaces: workspaces, nodes: nodes}
}

// homeCorrectedChats embeds the underlying ChatUsecase so every method
// passes through unchanged except the four read paths that hand back
// domain.Chat rows.
type homeCorrectedChats struct {
	ChatUsecase
	workspaces TreeWorkspaceGitStatus
	nodes      TreeNodes
}

func (u *homeCorrectedChats) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := u.ChatUsecase.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	return u.correctAll(ctx, rows), nil
}

func (u *homeCorrectedChats) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	rows, err := u.ChatUsecase.ListChatsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return u.correctAll(ctx, rows), nil
}

func (u *homeCorrectedChats) ListChatsInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	rows, err := u.ChatUsecase.ListChatsInRepo(ctx, repoID)
	if err != nil {
		return nil, err
	}
	return u.correctAll(ctx, rows), nil
}

func (u *homeCorrectedChats) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	row, err := u.ChatUsecase.GetChat(ctx, id)
	if err != nil {
		return domain.Chat{}, err
	}
	return u.correctOne(ctx, row), nil
}

func (u *homeCorrectedChats) correctAll(
	ctx context.Context,
	rows []domain.Chat,
) []domain.Chat {
	out := make([]domain.Chat, len(rows))
	for i, row := range rows {
		out[i] = u.correctOne(ctx, row)
	}
	return out
}

// NewHomeCorrectedTreeChats is NewHomeCorrectedChats' counterpart for the
// LOWER-level tree.Chats port — found while verifying this fix, not in the
// SDD review's own findings: NewChatLineage (container.go) is built
// DIRECTLY from the raw chat repository, entirely independent of
// Container.AgentChat/NewHomeCorrectedChats above, and internal/lineage's
// own Resolver.Ancestors reads chat.ParentID and every sibling row's
// ParentID straight off it (LoadChat/ListByWorkspace) with zero Node
// awareness — the SAME staleness bug as Critical 1, but for what a freshly
// spawned CLI is told to read (AssembleHandoff, the tool surface's
// Ancestors), not merely how the sidebar renders a row. A home-scoped
// thread moved under a different parent would hand its next spawn the
// WRONG (or missing) prior context.
//
// Wraps only the two reads internal/lineage.Chats actually needs
// (LoadChat, ListByWorkspace) — every other tree.Chats method (the mutating
// half: Create/SetTitle/SetPlacement/SetOrder/SetType/Forget, and the two
// other reads, Get/ListChats, unused by the lineage resolver) passes
// through via embedding.
func NewHomeCorrectedTreeChats(
	chats TreeChats,
	workspaces TreeWorkspaceGitStatus,
	nodes TreeNodes,
) TreeChats {
	return &homeCorrectedTreeChats{TreeChats: chats, workspaces: workspaces, nodes: nodes}
}

type homeCorrectedTreeChats struct {
	TreeChats
	workspaces TreeWorkspaceGitStatus
	nodes      TreeNodes
}

func (u *homeCorrectedTreeChats) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	row, err := u.TreeChats.LoadChat(ctx, id)
	if err != nil {
		return domain.Chat{}, err
	}
	return correctHomeChat(ctx, u.workspaces, u.nodes, row), nil
}

func (u *homeCorrectedTreeChats) ListByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	rows, err := u.TreeChats.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Chat, len(rows))
	for i, row := range rows {
		out[i] = correctHomeChat(ctx, u.workspaces, u.nodes, row)
	}
	return out, nil
}

// correctOne overlays row's live Node position when row is home-scoped —
// RepoOf answering "" for row.WorkspaceID is the SAME convention
// tree/internal's isHomeWorkspace uses. A row not yet minted onto Node (its
// very first placement, still mid-flight) or a repo-scoped/bubble row is
// returned unchanged.
func (u *homeCorrectedChats) correctOne(
	ctx context.Context,
	row domain.Chat,
) domain.Chat {
	return correctHomeChat(ctx, u.workspaces, u.nodes, row)
}

// correctHomeChat is the one small correction both decorators in this file
// share: overlay row's live Node position when row is home-scoped. RepoOf
// answering "" for row.WorkspaceID is the SAME convention tree/internal's
// own isHomeWorkspace uses. A row not yet minted onto Node (its very first
// placement, still mid-flight), a repo-scoped row, or a bubble
// (WorkspaceID == "") is returned unchanged.
func correctHomeChat(
	ctx context.Context,
	workspaces TreeWorkspaceGitStatus,
	nodes TreeNodes,
	row domain.Chat,
) domain.Chat {
	if row.WorkspaceID == "" {
		return row
	}
	repoID, err := workspaces.RepoOf(ctx, row.WorkspaceID)
	if err != nil || repoID != "" {
		return row
	}
	n, err := nodes.GetNode(ctx, row.ID)
	if err != nil {
		return row
	}
	row.ParentID = n.ParentID
	row.Order = n.Order
	return row
}
