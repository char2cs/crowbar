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
// the worktree.
//
// It is the fan-out set spec §7.4's shared bucket (git, review, files, search,
// identity) pushes to. Sibling chats on one worktree share its state, so they
// share its events: a write reached through ONE chat's route is visible to
// every chat currently resolving to that workspace, not just the one that
// triggered it.
//
// The forest is read and built ONCE per call and every row resolved against it
// through a shared memo, so the answer costs one ListChats rather than one per
// chat. Folder rows are never returned — a folder holds chats, it is not one,
// and nothing subscribes under a folder id.
//
// A workspace nobody currently points at yields an empty slice, not an error:
// having no subscribers is a fact, not a failure. So does an empty
// workspaceID, which must never be read as "every chat whose ancestry owns no
// worktree at all".
func ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
	chats ChatLister,
) ([]string, error) {
	if workspaceID == "" {
		return []string{}, nil
	}
	rows, err := chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("worktree: chats for workspace %s: list chats: %w", workspaceID, err)
	}
	forest := newChatForest(rows)
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
