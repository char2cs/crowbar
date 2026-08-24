package chatlineage

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chats is the chat read surface a lineage read needs: the one chat being asked
// about, and the workspace it belongs to.
//
// LoadChat and ListByWorkspace are NOT the same read, and the difference is the
// whole reason both are here. LoadChat folds the chat from the event log, so it
// is current the instant a write returns. ListByWorkspace serves the read-model
// projection, which is asynchronous — a chat placed a moment ago can still be
// listed at the placement it had before.
//
// The subject chat is the one that must not be read from the projection, because
// it is the row the operation that prompted this question just WROTE: a chat
// created under a parent, or dragged under one, is asked what it inherits within
// microseconds of its placement being written. Reading that from the projection
// returned an empty parent and injected no lineage at all, so a brand-new thread
// spent its first session not knowing it was one.
//
// The rows ABOVE it are read from the list, and safely: they are not what the
// operation wrote. What is left is a chat whose own parent was dragged in the
// same projection window as this read, which no single write can produce.
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
