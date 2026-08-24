package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CloseTurn struct {
	ChatID     string
	TurnID     string
	ProviderID string
	RunnerID   string
	SessionID  string
	Text       string
	Effort     string
	Now        time.Time
}

func (c CloseTurn) AggregateID() string  { return c.ChatID }
func (c CloseTurn) EventName() string    { return "agentactivity.turn_closed." + c.ChatID }
func (c CloseTurn) ShouldSnapshot() bool { return true }

func (c CloseTurn) Validate(*domain.ChatActivity) error {
	if err := requireChat("close turn", c.ChatID); err != nil {
		return err
	}
	return requireID("close turn", "turn id", c.TurnID)
}

func inheritOpenTurn(turn, open *domain.ActivityTurn) string {
	if open == nil {
		return ""
	}
	turn.StartedAt = open.StartedAt
	if turn.ProviderID == "" {
		turn.ProviderID = open.ProviderID
	}
	if turn.RunnerID == "" {
		turn.RunnerID = open.RunnerID
	}
	if turn.SessionID == "" {
		turn.SessionID = open.SessionID
	}
	return open.ID
}

func (c CloseTurn) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)

	turn := domain.ActivityTurn{
		ID:         c.TurnID,
		ChatID:     c.ChatID,
		Seq:        next.Seq,
		Role:       domain.TurnRoleAssistant,
		ProviderID: c.ProviderID,
		RunnerID:   c.RunnerID,
		SessionID:  c.SessionID,
		StartedAt:  c.Now,
	}

	superseded := inheritOpenTurn(&turn, next.Turn)
	turn.Text = c.Text
	if c.Effort != "" {
		turn.Effort = c.Effort
	}
	turn.EndedAt = at(c.Now)

	next.Turn = nil
	next.Tools = nil
	next.Subagents = nil
	next.Interruptions = nil
	next.Choices = nil
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
		SupersededTurnID: superseded,
	}
	return next
}
