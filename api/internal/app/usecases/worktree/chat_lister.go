package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatLister returns every chat and folder row across every workspace: walking ParentID
// edges upward needs the whole forest. The chat usecase's ListChats satisfies it.
type ChatLister interface {
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
}
