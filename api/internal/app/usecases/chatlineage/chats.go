package chatlineage

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chats is the chat read surface a lineage read needs: the one chat being asked
// about, and the workspace it belongs to.
//
// The single read is the AUTHORITATIVE one and the list is not, which is why
// both are here. The chat read model is an asynchronous projection, so a chat
// placed a moment ago can still be listed with the parent it had before; the
// keyed read heals exactly the row it was asked for. Ancestors therefore seeds
// the walk from the keyed read and lets the list answer only for the rows above
// it.
type Chats interface {
	GetChat(
		ctx context.Context,
		id string,
	) (domain.AgentChat, error)
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.AgentChat, error)
}
