package lineage

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chats is the read surface a lineage read needs. LoadChat folds from the event log and is current once a
// write returns; ListByWorkspace is the async projection. The subject, just written by the operation that
// asks, must come from LoadChat; its ancestors are safe to read from the list.
type Chats interface {
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}
