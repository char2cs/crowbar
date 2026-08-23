package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type SetSelection struct {
	ChatID string
	Model  string
	Effort string
}

func (c SetSelection) AggregateID() string  { return c.ChatID }
func (c SetSelection) EventName() string    { return "agentchat.selection_set." + c.ChatID }
func (c SetSelection) ShouldSnapshot() bool { return false }

func (c SetSelection) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("set selection: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetSelection) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.Model = c.Model
	next.Effort = c.Effort
	return next
}
