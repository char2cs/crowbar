package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Abandon closes whatever this chat left open, without inventing a reply.
//
// It is the reconcile path: a CLI that died mid-turn cannot still be working, and
// a turn left open would keep its chat spinning across every future boot. Nothing
// is fabricated — the turn is closed with the text it already had, which for an
// interrupted turn is none.
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
		// Nothing was open. The command still emits an event so the reconcile is
		// recorded, but it touches nothing.
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
