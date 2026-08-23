package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type InvokeTool struct {
	ChatID     string
	ToolID     string
	Name       string
	Target     string
	RequestRef string
	Now        time.Time
}

func (c InvokeTool) AggregateID() string  { return c.ChatID }
func (c InvokeTool) EventName() string    { return "agentactivity.tool_invoked." + c.ChatID }
func (c InvokeTool) ShouldSnapshot() bool { return false }

func (c InvokeTool) Validate(*domain.AgentActivity) error {
	if err := requireChat("invoke tool", c.ChatID); err != nil {
		return err
	}
	return requireID("invoke tool", "tool id", c.ToolID)
}

func (c InvokeTool) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)

	turnID := ensureTurn(&next, c.Now)

	call := domain.ActivityToolCall{
		ID:         c.ToolID,
		TurnID:     turnID,
		ChatID:     c.ChatID,
		Seq:        next.Seq,
		Name:       c.Name,
		Target:     c.Target,
		RequestRef: c.RequestRef,
		Status:     domain.ToolStatusRunning,
		StartedAt:  c.Now,
	}
	if !full(&next) {
		if next.Tools == nil {
			next.Tools = map[string]domain.ActivityToolCall{}
		}
		next.Tools[c.ToolID] = call
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaTool, Tool: &call,
	}
	return next
}

func ensureTurn(a *domain.AgentActivity, now time.Time) string {
	if a.Turn != nil {
		return a.Turn.ID
	}
	a.Turn = &domain.ActivityTurn{
		ID:        "turn-" + a.ChatID + "-" + itoa(a.Seq),
		ChatID:    a.ChatID,
		Seq:       a.Seq,
		Role:      domain.TurnRoleAssistant,
		StartedAt: now,
	}
	return a.Turn.ID
}

func currentTurn(a *domain.AgentActivity) string {
	if a.Turn == nil {
		return ""
	}
	return a.Turn.ID
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
