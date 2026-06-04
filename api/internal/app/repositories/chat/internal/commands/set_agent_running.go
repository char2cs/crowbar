package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetChatAgentRunning advances an idle Chat to agent-running.
type SetChatAgentRunning struct {
	ID string
}

func (c SetChatAgentRunning) AggregateID() string {
	return c.ID
}

func (c SetChatAgentRunning) EventName() string {
	return "chat.agent_running." + c.ID
}

func (c SetChatAgentRunning) ShouldSnapshot() bool {
	return false
}

func (c SetChatAgentRunning) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("set chat agent running: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetChatAgentRunning) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Status = domain.ChatStatusAgentRunning
	return chat
}
