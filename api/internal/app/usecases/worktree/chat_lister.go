package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ChatLister interface {
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
}
