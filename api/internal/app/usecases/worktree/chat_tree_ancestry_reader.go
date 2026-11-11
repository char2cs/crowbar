package worktree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// NewChatTreeAncestryReader climbs the chat/folder placement tree from chatID upward (CwdWorkspaceID's
// walk, which is internal to usecases/chat), keeping the intermediate chat rows Resolve needs, so a
// workspace-less chat under a folder reaches its worktree ancestor. nil folders/nodes walk chats only.
func NewChatTreeAncestryReader(
	lister ChatLister,
	folders Folders,
	nodes Nodes,
) ChatAncestryReader {
	return chatTreeAncestryReader{lister: lister, folders: folders, nodes: nodes}
}

type chatTreeAncestryReader struct {
	lister  ChatLister
	folders Folders
	nodes   Nodes
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
	return newChatForest(ctx, r.folders, r.nodes, rows).ancestry(chatID), nil
}
