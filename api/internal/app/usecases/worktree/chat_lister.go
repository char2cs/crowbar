package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatLister returns every chat and folder row this daemon knows, across every
// workspace. NewChatTreeAncestryReader climbs its ParentID edges, so it needs
// the whole forest rather than one chat's ancestors — a walk that starts from
// chatID cannot know which rows are above it without seeing the rest of the
// tree.
//
// Declared here rather than imported from usecases/chat (law 3, law 4):
// usecases/chat.Usecase.ListChats already returns exactly this shape.
type ChatLister interface {
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
}
