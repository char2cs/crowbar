package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CloseTurn completes the assistant turn with its hook-confirmed text.
//
// The closed turn keeps the id the CALLER supplied — the hook's delivery id —
// rather than the open turn's, so a redelivered reply rewrites one row instead of
// appending a second copy of what the agent said. The open turn's id travels on
// the delta as SupersededTurnID so the projection can re-point the tool calls that
// attached to it.
//
// A close with no open turn is not an error. Hooks arrive from a process Crowbar
// does not control, and a turn_stop that lands after a reconcile already closed
// the turn must not fail the hook — it records the reply on its own terms.
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

func (c CloseTurn) Validate(*domain.AgentActivity) error {
	if err := requireChat("close turn", c.ChatID); err != nil {
		return err
	}
	return requireID("close turn", "turn id", c.TurnID)
}

// inheritOpenTurn carries the open turn's start time and attribution onto the
// reply, and reports the placeholder id the reply supersedes. Each field is taken
// only where the close did not report one of its own.
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

func (c CloseTurn) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
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
	// Keep when the reply actually began, and its attribution, but not the
	// placeholder identity the open turn was minted under.
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
