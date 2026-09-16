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
// Widened in 2026-09-08 sidebar-placement-unification Task 8 from home-only
// to every non-bubble workspace: repo-scoped chat placement is Node-backed
// now too, and the identical staleness bug applies to a repo-scoped chat
// dragged into a repo-scoped folder. correctHomeChat no longer distinguishes
// scope at all — a bubble (WorkspaceID == "") is the only row left
// unchanged.
//
// NewHomeCorrectedChats wraps a ChatUsecase so every row it returns carries
// its LIVE position, overlaying the Node-sourced ParentID/Order the same way
// correctHomePlacement does — kept as an independent, small implementation
// rather than exported from tree/internal: this decorator's lifecycle
// (wraps the read surface every handler shares) is unrelated to a placement
// plan's, and the logic itself is five lines.
//
// This does NOT close the companion gap the SDD review also flagged: a WS
// broadcast for a chat's move (unlike a rename or a delete) only ever fires
// announceFolders, filtered to folder-kind rows — a live viewer's already-
// open session does not see the move until it re-fetches. Deliberately
// deferred: fixing it means teaching the hub's chat-lifecycle broadcast
// about Node moves, a materially larger change than this read-side fix.
func NewHomeCorrectedChats(
	chats ChatUsecase,
	nodes TreeNodes,
) ChatUsecase {
	return &homeCorrectedChats{ChatUsecase: chats, nodes: nodes}
}

// homeCorrectedChats embeds the underlying ChatUsecase so every method
// passes through unchanged except the four read paths that hand back
// domain.Chat rows.
type homeCorrectedChats struct {
	ChatUsecase
	nodes TreeNodes
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
// Ancestors), not merely how the sidebar renders a row. A thread moved
// under a different parent would hand its next spawn the WRONG (or
// missing) prior context.
//
// Wraps only the two reads internal/lineage.Chats actually needs
// (LoadChat, ListByWorkspace) — every other tree.Chats method (the mutating
// half: Create/SetTitle/SetPlacement/SetOrder/SetType/Forget, and the two
// other reads, Get/ListChats, unused by the lineage resolver) passes
// through via embedding.
//
// folders is a SECOND fix, 2026-09-08 sidebar-placement-unification Task 8:
// once a folder is Folder/Node-backed (never a Chat row — home-scoped since
// Task 5, repo-scoped too since Task 8), ListByWorkspace's raw read no
// longer carries it at ALL — internal/lineage.Resolver's own walk (Walk,
// lineage.go) steps THROUGH a folder ancestor by finding its ParentID in
// this same list, and a folder missing from the list breaks that walk
// exactly as if the chat sat at the panel root: a thread two folders under
// a real ancestor conversation would be told it has none. See
// foldersReachableFrom for how the gap is closed.
func NewHomeCorrectedTreeChats(
	chats TreeChats,
	nodes TreeNodes,
	folders TreeFolders,
) TreeChats {
	return &homeCorrectedTreeChats{TreeChats: chats, nodes: nodes, folders: folders}
}

type homeCorrectedTreeChats struct {
	TreeChats
	nodes   TreeNodes
	folders TreeFolders
}

func (u *homeCorrectedTreeChats) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	row, err := u.TreeChats.LoadChat(ctx, id)
	if err != nil {
		return domain.Chat{}, err
	}
	return correctHomeChat(ctx, u.nodes, row), nil
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
		out[i] = correctHomeChat(ctx, u.nodes, row)
	}
	folderRows, ferr := foldersReachableFrom(ctx, u.folders, u.nodes, out)
	if ferr != nil {
		// Degrades to the chat-only list rather than failing the whole
		// lineage read: a chat sitting directly under a real ancestor (no
		// folder between them, by far the common case) is unaffected, and a
		// missing folder discovery here is no worse than this package's own
		// pre-Task-8 posture, which had no folder awareness at all.
		return out, nil
	}
	return append(out, folderRows...), nil
}

// foldersReachableFrom discovers every Folder+Node row filed somewhere
// between workspaceRows (a workspace's own chats, already read) and the
// panel root, rendered as the same Chat-shaped view PlaceChat itself uses —
// exactly enough for internal/lineage.Resolver's walk to step THROUGH a
// folder ancestor rather than losing the thread the moment one sits between
// a chat and its real ancestor conversation. Home AND repo-scoped alike:
// nothing here checks Folder.RepoID, since the walk only cares whether an id
// resolves to SOMETHING it can keep climbing from, not which repo owns it.
//
// The BFS is seeded with every id workspaceRows already names (mirroring
// tree/internal's own mergeForest — see its doc for why: a folder nested
// under an ordinary chat, or under a branch's own owning row that has no
// Node of its own, is otherwise unreachable) plus the panel root, so a
// folder sitting at the bare root is found too.
func foldersReachableFrom(
	ctx context.Context,
	folders TreeFolders,
	nodes TreeNodes,
	workspaceRows []domain.Chat,
) ([]domain.Chat, error) {
	var found []domain.Chat
	queried := map[string]bool{}
	seenFolder := map[string]bool{}
	queue := make([]string, 0, len(workspaceRows)+1)
	queue = append(queue, "")
	for _, row := range workspaceRows {
		queue = append(queue, row.ID)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if queried[parent] {
			continue
		}
		queried[parent] = true
		children, err := nodes.ListByParent(ctx, parent)
		if err != nil {
			return nil, err
		}
		for _, n := range children {
			if n.Kind != domain.NodeKindFolder || seenFolder[n.ID] {
				continue
			}
			seenFolder[n.ID] = true
			f, ferr := folders.FindByKey(ctx, n.ID)
			if ferr != nil {
				return nil, ferr
			}
			if f == nil {
				continue
			}
			found = append(found, domain.Chat{
				ID: f.ID, Type: domain.ChatTypeFolder, RepoID: f.RepoID,
				Title: f.Name, ParentID: n.ParentID, Order: n.Order,
			})
			if !queried[n.ID] {
				queue = append(queue, n.ID)
			}
		}
	}
	return found, nil
}

// correctOne overlays row's live Node position. A row not yet minted onto
// Node (its very first placement, still mid-flight) or a bubble
// (WorkspaceID == "") is returned unchanged.
func (u *homeCorrectedChats) correctOne(
	ctx context.Context,
	row domain.Chat,
) domain.Chat {
	return correctHomeChat(ctx, u.nodes, row)
}

// correctHomeChat is the one small correction both decorators in this file
// share: overlay row's live Node position. Widened in 2026-09-08
// sidebar-placement-unification Task 8 from home-only to every non-bubble
// workspace — repo-scoped chat placement is Node-backed now too, and the
// SAME staleness bug Critical 1 caught for home applies identically to a
// repo-scoped chat dragged into a repo-scoped folder: GET /chats would have
// rendered it at the wrong position forever otherwise. A row not yet minted
// onto Node (its very first placement, still mid-flight — GetNode's error
// case) or a bubble (WorkspaceID == "") is returned unchanged.
func correctHomeChat(
	ctx context.Context,
	nodes TreeNodes,
	row domain.Chat,
) domain.Chat {
	if row.WorkspaceID == "" {
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
