// Package worktree resolves a chat to the workspace whose worktree it reads and writes
// through: the single seam every chat-scoped route uses in place of a public workspace id.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ErrNoWorktreeInAncestry is returned when neither chatID nor any chat in its ancestry
// owns a worktree.
var ErrNoWorktreeInAncestry = errors.New("worktree: no chat in ancestry owns a worktree")

// Resolve finds the workspace behind chatID's worktree: its own, else the nearest
// ancestor's. No worktree in the ancestry returns ErrNoWorktreeInAncestry, never a zero value.
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

// ChatsForWorkspace is Resolve's inverse: the sorted chat ids (never folders) resolving to
// workspaceID, the census a cascading delete checks first. An unreferenced or empty id
// yields an empty slice; nil folders/nodes degrade to a chat-only walk.
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
