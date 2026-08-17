package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenTurn starts the assistant turn that tool calls, subagents and
// interruptions attach to.
//
// It supersedes any turn already open. A turn that was never closed cannot be
// waiting for anything — the CLI is demonstrably talking again — and an
// interrupted turn is closed by nothing at all, because an ESC fires no hook on
// either provider.
type OpenTurn struct {
	ChatID     string
	TurnID     string
	ProviderID string
	RunnerID   string
	SessionID  string
	Now        time.Time
}

func (c OpenTurn) AggregateID() string  { return c.ChatID }
func (c OpenTurn) EventName() string    { return "agentactivity.turn_opened." + c.ChatID }
func (c OpenTurn) ShouldSnapshot() bool { return true }

func (c OpenTurn) Validate(*domain.AgentActivity) error {
	if err := requireChat("open turn", c.ChatID); err != nil {
		return err
	}
	return requireID("open turn", "turn id", c.TurnID)
}

func (c OpenTurn) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	// Whatever the last turn left in flight belongs to that turn, not this one.
	next.Tools = nil
	next.Subagents = nil
	next.Interruptions = nil
	next.Turn = &domain.ActivityTurn{
		ID:         c.TurnID,
		ChatID:     c.ChatID,
		Seq:        next.Seq,
		Role:       domain.TurnRoleAssistant,
		ProviderID: c.ProviderID,
		RunnerID:   c.RunnerID,
		SessionID:  c.SessionID,
		StartedAt:  c.Now,
	}
	turn := *next.Turn
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaTurn, Turn: &turn,
	}
	return next
}
