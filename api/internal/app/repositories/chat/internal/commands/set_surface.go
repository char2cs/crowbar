package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetSurface moves a chat to the view the user is on now, keeping
// domain.Chat.Surface a current fact rather than a birth record.
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
	// "" is accepted: it is the provider's own default landing, a real place
	// to move back to, not a missing value.
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
