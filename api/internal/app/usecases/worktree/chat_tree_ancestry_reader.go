package worktree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
