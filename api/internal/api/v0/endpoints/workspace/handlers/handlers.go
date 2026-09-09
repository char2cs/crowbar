// Package handlers serves the workspace-addressed placement route: PATCH
// .../workspaces/:wsId/placement, the first genuinely workspace-native route
// this daemon has grown since spec §8 moved everything else onto the
// chat-addressed surface (see router.go's own doc — threads is the only
// other member of the "/workspaces/:wsId/..." mount off repoScoped, and this
// is the second, for the identical reason: a LOCKED branch's own sidebar
// position is a fact about the workspace itself, not about any conversation
// living inside it, and locked workspaces are not chats (2026-09-09
// sidebar-placement-unification, workspace-placement fix).
package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Placer is the narrow write port this handler needs off the sidebar tree
// usecase — PlaceWorkspace alone, satisfied structurally by
// usecases/chat.TreeUsecase, the same concrete value chat.Register's own
// ChatTreeUsecase port is already handed (container.go's AgentChatFolder).
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

// New builds the workspace Handlers over the tree usecase's PlaceWorkspace
// surface.
func New(
	placer Placer,
) *Handlers {
	return &Handlers{placer: placer}
}
