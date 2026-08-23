package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type Abandon struct {
	ChatID string
	Now    time.Time
}

func (c Abandon) AggregateID() string  { return c.ChatID }
func (c Abandon) EventName() string    { return "agentactivity.abandoned." + c.ChatID }
func (c Abandon) ShouldSnapshot() bool { return true }

func (c Abandon) Validate(*domain.AgentActivity) error {
	return requireChat("abandon", c.ChatID)
}

func (c Abandon) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	if next.Turn == nil {

		return next
	}
	turn := *next.Turn
	turn.EndedAt = at(c.Now)
	next.Turn = nil
	next.Tools = nil
	next.Subagents = nil
	next.Interruptions = nil
	next.Choices = nil
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
		SupersededTurnID: turn.ID,
	}
	return next
}
