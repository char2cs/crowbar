package chatlineage

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Folders is the chat-folder read surface: one workspace's folder rows, needed
// only so the walk can step THROUGH them. Nothing here ever collects a folder —
// they hold no turns — but a lineage that could not resolve a folder's own
// parent would stop dead at the first one and report that a filed thread
// inherits nothing.
type Folders interface {
	FindWhere(
		ctx context.Context,
		match domain.AgentChatFolder,
	) ([]domain.AgentChatFolder, error)
}
