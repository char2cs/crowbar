package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatAncestryReader returns chatID's ancestry as full chat rows, chatID
// ITSELF first, then each parent in turn, nearest first.
//
// This is a different contract from usecases/chat.Usecase.Ancestors, which
// returns parent IDs only, excluding the subject, filtered to CHAT-typed rows,
// and scoped to the subject's OWN workspace — that method answers "what does a
// thread inherit," not "what worktree does this chat resolve to," and is empty
// for exactly the chat Resolve most needs to walk past: a workspace-less chat
// filed under a folder whose worktree-owning ancestor sits above it.
// NewChatTreeAncestryReader (in this package) is the real implementation,
// climbing the placement tree directly instead; declared here rather than
// imported from usecases/chat (law 3, law 4).
type ChatAncestryReader interface {
	Ancestors(ctx context.Context, chatID string) ([]domain.Chat, error)
}
