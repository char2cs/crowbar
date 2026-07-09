package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Delete tombstones an AgentChat by setting Status=deleted.
type Delete struct {
	ChatID string
}

func (c Delete) AggregateID() string  { return c.ChatID }
func (c Delete) EventName() string    { return "agentchat.deleted." + c.ChatID }
func (c Delete) ShouldSnapshot() bool { return false }

func (c Delete) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("delete chat: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Delete) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.Status = domain.AgentChatStatusDeleted
	return next
}
