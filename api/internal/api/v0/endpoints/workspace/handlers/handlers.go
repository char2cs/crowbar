// Package handlers serves PATCH .../workspaces/:wsId/placement. It is workspace-addressed rather
// than chat-addressed because a locked branch's sidebar position is a fact about the workspace,
// and locked workspaces are not chats.
package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Placer is the sidebar tree usecase's PlaceWorkspace port, satisfied by usecases/chat.TreeUsecase.
type Placer interface {
	PlaceWorkspace(
		ctx context.Context,
		workspaceID string,
		in agentusecase.PlaceInput,
	) (domain.Chat, []domain.Chat, error)
}

// Handlers serves the workspace placement route.
type Handlers struct {
	placer Placer
}

// New builds the workspace Handlers over the tree usecase's PlaceWorkspace surface.
func New(placer Placer) *Handlers {
	return &Handlers{placer: placer}
}
