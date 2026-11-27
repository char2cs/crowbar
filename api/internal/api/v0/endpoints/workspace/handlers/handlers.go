package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Placer interface {
	PlaceWorkspace(
		ctx context.Context,
		workspaceID string,
		in agentusecase.PlaceInput,
	) (domain.Chat, []domain.Chat, error)
}

type Handlers struct {
	placer Placer
}

func New(placer Placer) *Handlers {
	return &Handlers{placer: placer}
}
