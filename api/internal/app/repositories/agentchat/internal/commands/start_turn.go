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
	next.CurrentTurnStarted = &t
	// A new turn SUPERSEDES whatever the last one left outstanding. The CLI is
	// talking again, and its next turn_stop will restate the level from scratch —
	// so carrying the old number forward could only ever preserve a ghost.
	//
	// This is the self-healing edge that covers the one case the hook surface cannot
	// report at all: an INTERRUPT (ESC) fires NO hook whatsoever — not Stop, not
	// Notification, nothing (measured against claude 2.1.212) — so a turn the user
	// interrupted is never closed by anything, and any level it left standing would
	// sit there with it. The next prompt clears both.
	next.AsyncWork = 0
	next.Working = foldWorking(&next)
	next.LastActivityAt = c.Now
	return next
}
