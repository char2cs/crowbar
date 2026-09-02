package worktree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatAncestryReader returns chatID's ancestry as full chat rows, chatID
// ITSELF first, then each parent in turn, nearest first.
//
// This is a different contract from usecases/chat.Usecase.Ancestors, which
// returns parent IDs only, excluding the subject — that method answers "what
// does a thread inherit," not "what worktree does this chat resolve to," and
// Resolve needs the subject's own WorkspaceID to short-circuit the
// owns-its-own-worktree case. The container's adapter composes the two calls
// that exist today (GetChat + Ancestors) into this shape; declared here
// rather than imported from usecases/chat (law 3, law 4).
type ChatAncestryReader interface {
	Ancestors(ctx context.Context, chatID string) ([]domain.Chat, error)
}
