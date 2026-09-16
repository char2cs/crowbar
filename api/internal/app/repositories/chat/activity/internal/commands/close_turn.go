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
	// ItemIndex is this message's position within its turn — see
	// ActivityTurn.ItemIndex. Zero for every turn but a multi-item one
	// (Codex splitting a reply across several message ids).
	ItemIndex int
	Now       time.Time
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
		ID:     c.TurnID,
		ChatID: c.ChatID,
		Seq:    next.Seq,
		// Fallback only — overwritten below whenever this runner reserved a
		// DisplayOrder at OpenTurn's dispatch time. Matches today's behavior
		// for the rare case where none exists
		// (TestCloseTurn_WithNoOpenTurnStillRecordsTheReply).
		DisplayOrder: next.Seq,
		ItemIndex:    c.ItemIndex,
		Role:         domain.TurnRoleAssistant,
		ProviderID:   c.ProviderID,
		RunnerID:     c.RunnerID,
		SessionID:    c.SessionID,
		StartedAt:    c.Now,
	}

	// Looked up by RunnerID, not by matching next.Turn positionally: Turn is
	// a single pointer that a DIFFERENT runner's OpenTurn can — and, in the
	// exact scenario this exists for, DOES — replace before this runner's
	// own turn ever gets a chance to close. OpenTurnOrders survives that
	// replacement because nothing but this lookup ever clears it.
	if reserved, ok := next.OpenTurnOrders[c.RunnerID]; ok {
		turn.DisplayOrder = reserved
		delete(next.OpenTurnOrders, c.RunnerID)
	}

	// THE REGRESSION. next.Turn is that same single pointer, and inheriting
	// and consuming it unconditionally trusted whoever happened to be sitting
	// there — so the exact replacement the comment above names (a different
	// runner's OpenTurn winning the slot before this runner's own turn could
	// close) made this close SUPERSEDE and REPOINT the OTHER runner's still-
	// live tools, subagents, interruptions and choices onto ITS OWN message.
	// Live symptom: a pile of an unrelated turn's tool rows — a provider
	// switch's outgoing CLI, or a subagent's own runner racing this chat's
	// activity — landing under the NEXT reply, sometimes hundreds of them,
	// because they had silently piled up under a Turn this close never owned.
	//
	// An empty RunnerID on either side still owns it: InvokeTool's own
	// no-open-turn fallback (ensureTurn) mints a Turn with none set at all,
	// and that turn's own eventual close must still claim and repoint it —
	// see TestInvokeTool_WithNoOpenTurnOpensOneImplicitly and
	// TestCloseTurn_WithNoOpenTurnStillRecordsTheReply. Only a turn NAMED for
	// a runner other than this one is foreign.
	var superseded string
	ownsCurrent := next.Turn != nil &&
		(c.RunnerID == "" || next.Turn.RunnerID == "" || next.Turn.RunnerID == c.RunnerID)
	if ownsCurrent {
		superseded = inheritOpenTurn(&turn, next.Turn)
		next.Turn = nil
		next.Tools = nil
		next.Subagents = nil
		next.Interruptions = nil
		next.Choices = nil
	}
	turn.Text = c.Text
	if c.Effort != "" {
		turn.Effort = c.Effort
	}
	turn.EndedAt = at(c.Now)

	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
		SupersededTurnID: superseded,
	}
	return next
}
