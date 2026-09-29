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
	CarryPlan  bool
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
	var plan []domain.ActivityPlanStep
	var planUpdatedAt *time.Time
	if c.CarryPlan && current != nil && current.Last != nil && current.Last.Turn != nil &&
		(c.RunnerID == "" || current.Last.Turn.RunnerID == "" || current.Last.Turn.RunnerID == c.RunnerID) {
		plan = append([]domain.ActivityPlanStep(nil), current.Last.Turn.Plan...)
		planUpdatedAt = current.Last.Turn.PlanUpdatedAt
	} else if next.Turn != nil &&
		(c.RunnerID == "" || next.Turn.RunnerID == "" || next.Turn.RunnerID == c.RunnerID) {
		plan = append([]domain.ActivityPlanStep(nil), next.Turn.Plan...)
		planUpdatedAt = next.Turn.PlanUpdatedAt
	}

	next.Tools = nil
	next.Subagents = nil
	next.Interruptions = nil
	next.Choices = nil
	if next.OpenTurnOrders == nil {
		next.OpenTurnOrders = map[string]int64{}
	}
	next.OpenTurnOrders[c.RunnerID] = next.Seq
	next.Turn = &domain.ActivityTurn{
		ID:     c.TurnID,
		ChatID: c.ChatID,
		Seq:    next.Seq,
		// Reserved HERE, at true dispatch time — see ActivityTurn.DisplayOrder.
		DisplayOrder:  next.Seq,
		Role:          domain.TurnRoleAssistant,
		ProviderID:    c.ProviderID,
		RunnerID:      c.RunnerID,
		SessionID:     c.SessionID,
		Status:        "active",
		Plan:          plan,
		PlanUpdatedAt: planUpdatedAt,
		StartedAt:     c.Now,
	}
	turn := *next.Turn
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaTurn, Turn: &turn,
	}
	return next
}
