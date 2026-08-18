package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetSelection writes the chat's sticky choice of model and reasoning effort.
//
// Both fields are written TOGETHER and unconditionally, including to empty: an
// empty value is the user asking for the provider's default back, so a command
// that skipped empties could never clear a choice.
//
// It validates nothing about the values themselves. Whether an id exists in a
// provider's catalogue is a descriptor question, and the descriptor lives in the
// engine — the aggregate would have to import it to ask, which is exactly the
// dependency the repository layer must not have. The usecase validates before it
// sends.
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
