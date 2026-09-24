package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetProvider restates the VENDOR a chat runs as — see domain.Chat.ProviderID.
//
// Create seeds the same field; this is what keeps it CURRENT, and its callers
// are every moment a CLI is actually placed on the chat (a spawn, a switch, a
// runner moved onto it by its own /clear). It is a record of what happened, not
// a decision: nothing here may invent a provider for a chat that has none.
type SetProvider struct {
	ChatID     string
	ProviderID string
}

func (c SetProvider) AggregateID() string  { return c.ChatID }
func (c SetProvider) EventName() string    { return "agentchat.provider_set." + c.ChatID }
func (c SetProvider) ShouldSnapshot() bool { return false }

func (c SetProvider) Validate(current *domain.Chat) error {
	if current == nil {
		return fmt.Errorf("set provider: no chat: %w", asynxModels.ErrValidation)
	}
	// "" is refused, unlike SetSurface's: an empty surface is a real place (the
	// provider's own landing), while an empty provider is only ever the absence
	// this field exists to end — and clearing it would put the chat straight
	// back into the state that let a reopen guess one.
	if c.ProviderID == "" {
		return fmt.Errorf("set provider: empty provider: %w", asynxModels.ErrValidation)
	}
	if current.ProviderID == c.ProviderID {
		return fmt.Errorf("set provider: unchanged: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetProvider) EmitEvent(current *domain.Chat) domain.Chat {
	next := *current
	next.ProviderID = c.ProviderID
	return next
}
