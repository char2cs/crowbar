package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatAncestryReader returns chatID's ancestry as full rows, itself first, nearest first.
// Unlike the chat usecase's Ancestors (chat-typed parents within one workspace), it climbs
// past folders, so a workspace-less chat filed under one still reaches its worktree owner.
type ChatAncestryReader interface {
	Ancestors(ctx context.Context, chatID string) ([]domain.Chat, error)
}
