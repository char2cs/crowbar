package worktree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// NewChatTreeAncestryReader adapts a ChatLister into a ChatAncestryReader by
// climbing the chat/folder placement tree from chatID upward — the same walk
// usecases/chat/internal/tree.CwdWorkspaceID runs to answer "where does this
// row's CLI run," ported onto the generic tree engine here because that
// package is internal to usecases/chat and Resolve needs the intermediate
// chat ROWS, not just the workspace id CwdWorkspaceID stops at.
//
// It replaces resolving chatID's ancestry through usecases/chat.Usecase
// .Ancestors, which answers a different question — conversation inheritance,
// scoped to chatID's OWN workspace — and is empty for exactly the chat this
// reader must resolve: one with no workspace of its own, filed under a
// folder, whose worktree-owning ancestor sits above it.
func NewChatTreeAncestryReader(
	lister ChatLister,
) ChatAncestryReader {
	return chatTreeAncestryReader{lister: lister}
}

type chatTreeAncestryReader struct {
	lister ChatLister
}

// Ancestors implements ChatAncestryReader: chatID itself first, then each
// parent in turn, nearest first, stopping at the first row that carries a
// WorkspaceID or once the tree runs out of parents.
func (r chatTreeAncestryReader) Ancestors(
	ctx context.Context,
	chatID string,
) ([]domain.Chat, error) {
	rows, err := r.lister.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat ancestry: list chats: %w", err)
	}
	byID := make(map[string]domain.Chat, len(rows))
	nodes := make([]tree.Node, len(rows))
	for i, row := range rows {
		byID[row.ID] = row
		nodes[i] = tree.Node{ID: row.ID, ParentID: row.ParentID}
	}
	forest := tree.New(nodes)
	ancestry := make([]domain.Chat, 0, 4)
	seen := map[string]bool{}
	for id := chatID; id != "" && !seen[id]; {
		seen[id] = true
		row, ok := byID[id]
		if !ok {
			break
		}
		ancestry = append(ancestry, row)
		if row.WorkspaceID != "" {
			break
		}
		node, ok := forest.Node(id)
		if !ok {
			break
		}
		id = node.ParentID
	}
	return ancestry, nil
}
