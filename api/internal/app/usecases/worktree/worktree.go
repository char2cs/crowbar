package worktree

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var ErrNoWorktreeInAncestry = errors.New("worktree: no chat in ancestry owns a worktree")

func Resolve(
	ctx context.Context,
	chatID string,
	chats ChatAncestryReader,
	workspaces WorkspaceReader,
) (domain.Workspace, error) {
	ancestry, err := chats.Ancestors(ctx, chatID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("worktree: resolve %s: ancestry: %w", chatID, err)
	}
	for _, c := range ancestry {
		if c.WorkspaceID == "" {
			continue
		}
		ws, err := workspaces.Get(ctx, c.WorkspaceID)
		if err != nil {
			return domain.Workspace{}, fmt.Errorf("worktree: resolve %s: workspace %s: %w", chatID, c.WorkspaceID, err)
		}
		return ws, nil
	}
	return domain.Workspace{}, ErrNoWorktreeInAncestry
}

func ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
	chats ChatLister,
	folders Folders,
	nodes Nodes,
) ([]string, error) {
	if workspaceID == "" {
		return []string{}, nil
	}
	rows, err := chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("worktree: chats for workspace %s: list chats: %w", workspaceID, err)
	}
	forest := newChatForest(ctx, folders, nodes, rows)
	memo := make(map[string]string, len(rows))
	chatIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Type == domain.ChatTypeFolder {
			continue
		}
		if forest.workspaceFor(row.ID, memo) == workspaceID {
			chatIDs = append(chatIDs, row.ID)
		}
	}
	slices.Sort(chatIDs)
	return chatIDs, nil
}
