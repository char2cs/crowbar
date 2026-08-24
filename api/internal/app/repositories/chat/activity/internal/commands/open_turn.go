package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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

func (c OpenTurn) Validate(*domain.ChatActivity) error {
	if err := requireChat("open turn", c.ChatID); err != nil {
		return err
	}
	return requireID("open turn", "turn id", c.TurnID)
}

func (c OpenTurn) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)

	next.Tools = nil
	next.Subagents = nil
	next.Interruptions = nil
	next.Choices = nil
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
