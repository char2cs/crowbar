// Package worktree resolves a chat to the workspace whose worktree it reads
// and writes through — the single seam every chat-scoped route (spec
// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §3) uses in
// place of a public workspaceId.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ErrNoWorktreeInAncestry is returned when chatID owns no worktree, and
// neither does any chat in its ancestry — a bubble hanging off nothing, per
// spec §3, should that ever be reachable.
var ErrNoWorktreeInAncestry = errors.New("worktree: no chat in ancestry owns a worktree")

// Resolve finds the workspace behind chatID's worktree: chatID's own, if it
// owns one, else the nearest ancestor's, itself first and each parent in
// turn, nearest first. A chat with no worktree anywhere in its ancestry
// returns ErrNoWorktreeInAncestry, never a zero domain.Workspace with a nil
// error.
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

// ChatsForWorkspace is Resolve's inverse: every chat id whose nearest
// worktree-owning ancestor is workspaceID, sorted, itself included when it owns
// the worktree — the census a cascading delete checks before reaping it.
//
// The forest is read once per call. Folder rows are never returned, and a
// workspace nobody points at (or an empty workspaceID) yields an empty slice,
// not an error. folders/nodes let the walk cross a Folder-only ancestor; either
// may be nil, degrading to the Chat-only walk.
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

// WorkspacesForChats answers Resolve for many chats from one forest read, keyed
// by chat id; a folder, an unknown id or a chat with no worktree in its
// ancestry answers "". It is what a rebind of every chat-scoped stream costs.
func WorkspacesForChats(
	ctx context.Context,
	chatIDs []string,
	chats ChatLister,
	folders Folders,
	nodes Nodes,
) (map[string]string, error) {
	rows, err := chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("worktree: workspaces for %d chats: list chats: %w", len(chatIDs), err)
	}
	forest := newChatForest(ctx, folders, nodes, rows)
	memo := make(map[string]string, len(rows))
	out := make(map[string]string, len(chatIDs))
	for _, id := range chatIDs {
		if row, ok := forest.byID[id]; !ok || row.Type == domain.ChatTypeFolder {
			out[id] = ""
			continue
		}
		out[id] = forest.workspaceFor(id, memo)
	}
	return out, nil
}
