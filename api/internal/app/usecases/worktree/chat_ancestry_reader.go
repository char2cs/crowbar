package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ChatAncestryReader interface {
	Ancestors(ctx context.Context, chatID string) ([]domain.Chat, error)
}
