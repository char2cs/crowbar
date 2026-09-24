package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetSurface moves a chat to the VIEW the user is on now — see
// domain.Chat.Surface, which is the single source of truth for that.
//
// Create seeds the same field; this is what makes it a CURRENT fact rather
// than a birth record, and the reason SwitchToTerminal/SwitchToNative are the
// only callers: they are the two moments Crowbar is actually told.
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
