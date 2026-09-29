package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// UpdatePlan replaces the open turn's plan wholesale. Plan hooks are
// snapshots, not deltas: an empty Steps list deliberately clears an earlier
// plan. The command snapshots so the latest plan remains part of the open
// aggregate across a daemon restart before the turn closes.
type UpdatePlan struct {
	ChatID     string
	ProviderID string
	RunnerID   string
	SessionID  string
	Steps      []domain.ActivityPlanStep
	Now        time.Time
}

func (c UpdatePlan) AggregateID() string  { return c.ChatID }
func (c UpdatePlan) EventName() string    { return "agentactivity.plan_updated." + c.ChatID }
func (c UpdatePlan) ShouldSnapshot() bool { return true }

func (c UpdatePlan) Validate(*domain.ChatActivity) error {
	return requireChat("update plan", c.ChatID)
}

func (c UpdatePlan) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	if next.Turn == nil {
		ensureTurn(&next, c.Now)
		next.Turn.ProviderID = c.ProviderID
		next.Turn.RunnerID = c.RunnerID
		next.Turn.SessionID = c.SessionID
	}
	// A late hook from a gracefully stopping runner must not replace the plan
	// of a different runner that now owns the chat's open turn.
	if c.RunnerID != "" && next.Turn.RunnerID != "" && next.Turn.RunnerID != c.RunnerID {
		return next
	}
	steps := append([]domain.ActivityPlanStep(nil), c.Steps...)
	turn := *next.Turn
	turn.Plan = steps
	turn.PlanUpdatedAt = at(c.Now)
	next.Turn = &turn
	return next
}
