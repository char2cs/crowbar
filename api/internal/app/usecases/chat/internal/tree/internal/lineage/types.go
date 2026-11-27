package lineage

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
