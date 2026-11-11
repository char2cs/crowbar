package chat

import (
	"context"

	apptree "github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// NewHomeCorrectedChats wraps a ChatUsecase so every row it returns carries its LIVE
// Node position: a non-bubble chat's own ParentID/Order are written once at creation
// and then ignored, since Node is the position authority.
func NewHomeCorrectedChats(
	chats ChatUsecase,
	nodes TreeNodes,
) ChatUsecase {
	return &homeCorrectedChats{ChatUsecase: chats, nodes: nodes}
}

// homeCorrectedChats passes every method through except the four reads that
// return domain.Chat rows.
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
	return correctHomeChats(ctx, u.nodes, rows)
}

// NewHomeCorrectedTreeChats corrects the two reads the lineage resolver makes
// (LoadChat, ListByWorkspace) to live Node positions, and adds the folders the walk
// must step through, so a moved thread's next spawn gets the right prior context.
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
		// Degrade to the chat-only list rather than fail the lineage read: a chat
		// directly under its ancestor (the common case) is unaffected.
		return out, nil
	}
	return append(out, folderRows...), nil
}

// foldersReachableFrom is every folder between workspaceRows and the panel root.
func foldersReachableFrom(
	ctx context.Context, folders TreeFolders, nodes TreeNodes, workspaceRows []domain.Chat,
) ([]domain.Chat, error) {
	byParent, err := childrenByParent(ctx, nodes)
	if err != nil {
		return nil, err
	}
	return apptree.FoldersBelow(ctx, folders, byParent, workspaceRows)
}

// correctOne overlays row's live Node position.
func (u *homeCorrectedChats) correctOne(
	ctx context.Context,
	row domain.Chat,
) domain.Chat {
	return correctHomeChat(ctx, u.nodes, row)
}

// correctHomeChat overlays row's live Node position, for every non-bubble chat. A
// bubble, or a row with no Node yet (first placement still mid-flight), is unchanged.
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

// correctHomeChats is correctHomeChat for a list, from one read of the Node table; a
// lone row keeps the cheaper by-id lookup, and a failed read leaves every row as it was.
func correctHomeChats(
	ctx context.Context,
	nodes TreeNodes,
	rows []domain.Chat,
) []domain.Chat {
	out := make([]domain.Chat, len(rows))
	copy(out, rows)
	if len(out) == 0 {
		return out
	}
	if len(out) == 1 {
		out[0] = correctHomeChat(ctx, nodes, out[0])
		return out
	}
	all, err := nodes.ListAll(ctx)
	if err != nil {
		return out
	}
	byID := make(map[string]tree.Node, len(all))
	for _, n := range all {
		byID[n.ID] = n
	}
	for i, row := range out {
		if n, ok := byID[row.ID]; ok && row.WorkspaceID != "" {
			out[i].ParentID, out[i].Order = n.ParentID, n.Order
		}
	}
	return out
}

// childrenByParent is every node under each parent, from one read of them all:
// a walk asks for many levels, and each read decodes every project's Tree.
func childrenByParent(ctx context.Context, nodes TreeNodes) (map[string][]tree.Node, error) {
	all, err := nodes.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]tree.Node{}
	for _, n := range all {
		out[n.ParentID] = append(out[n.ParentID], n)
	}
	return out, nil
}
