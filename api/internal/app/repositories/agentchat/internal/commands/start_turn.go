package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// StartTurn marks the chat as live (Working) at the start of an agent turn.
type StartTurn struct {
	ChatID string
	Now    time.Time
}

func (c StartTurn) AggregateID() string  { return c.ChatID }
func (c StartTurn) EventName() string    { return "agentchat.turn_started." + c.ChatID }
func (c StartTurn) ShouldSnapshot() bool { return false }

func (c StartTurn) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("start turn: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c StartTurn) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	t := c.Now
	next.Working = true
	next.CurrentTurnStarted = &t
	next.LastActivityAt = c.Now
	return next
}
