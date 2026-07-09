package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// StopTurn clears the live (Working) flag at the end of an agent turn.
type StopTurn struct {
	ChatID string
	Now    time.Time
}

func (c StopTurn) AggregateID() string  { return c.ChatID }
func (c StopTurn) EventName() string    { return "agentchat.turn_stopped." + c.ChatID }
func (c StopTurn) ShouldSnapshot() bool { return false }

func (c StopTurn) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("stop turn: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c StopTurn) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.Working = false
	next.CurrentTurnStarted = nil
	next.LastActivityAt = c.Now
	return next
}
