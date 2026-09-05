package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type AppendTurn struct {
	ChatID     string
	TurnID     string
	Role       string
	ProviderID string
	RunnerID   string
	SessionID  string
	Text       string
	Effort     string
	Now        time.Time
}

func (c AppendTurn) AggregateID() string  { return c.ChatID }
func (c AppendTurn) EventName() string    { return "agentactivity.turn_appended." + c.ChatID }
func (c AppendTurn) ShouldSnapshot() bool { return true }

func (c AppendTurn) Validate(*domain.ChatActivity) error {
	if err := requireChat("append turn", c.ChatID); err != nil {
		return err
	}
	if err := requireID("append turn", "turn id", c.TurnID); err != nil {
		return err
	}
	if !domain.KnownTurnRole(c.Role) {
		return fmt.Errorf("append turn: unknown role %q: %w", c.Role, asynxModels.ErrValidation)
	}
	return nil
}

func (c AppendTurn) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	turn := domain.ActivityTurn{
		ID:     c.TurnID,
		ChatID: c.ChatID,
		Seq:    next.Seq,
		// One synchronous record, no open/close cycle — next.Seq is already
		// reserved at genuine dispatch time, unlike CloseTurn's.
		DisplayOrder: next.Seq,
		Role:         c.Role,
		ProviderID:   c.ProviderID,
		RunnerID:     c.RunnerID,
		SessionID:    c.SessionID,
		Text:         c.Text,
		Effort:       c.Effort,
		StartedAt:    c.Now,
		EndedAt:      at(c.Now),
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
	}
	return next
}
