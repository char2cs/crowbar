package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SetSurface struct {
	ChatID  string
	Surface string
}

func (c SetSurface) AggregateID() string  { return c.ChatID }
func (c SetSurface) EventName() string    { return "agentchat.surface_set." + c.ChatID }
func (c SetSurface) ShouldSnapshot() bool { return false }

func (c SetSurface) Validate(current *domain.Chat) error {
	if current == nil {
		return fmt.Errorf("set surface: no chat: %w", asynxModels.ErrValidation)
	}
	if !domain.KnownSurface(c.Surface) {
		return fmt.Errorf("set surface: unknown surface %q: %w", c.Surface, asynxModels.ErrValidation)
	}
	return nil
}

func (c SetSurface) EmitEvent(current *domain.Chat) domain.Chat {
	next := *current
	next.Surface = c.Surface
	return next
}
