package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
