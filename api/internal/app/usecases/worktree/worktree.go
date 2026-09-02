// Package worktree resolves a chat to the workspace whose worktree it reads
// and writes through — the single seam every chat-scoped route (spec
// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §3) uses in
// place of a public workspaceId.
package worktree

import (
	"context"
	"errors"
	"fmt"

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
