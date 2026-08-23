package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type CompleteTool struct {
	ChatID     string
	ToolID     string
	Name       string
	Target     string
	ResultRef  string
	Status     string
	Error      string
	DurationMS int
	Now        time.Time
}

func (c CompleteTool) AggregateID() string  { return c.ChatID }
func (c CompleteTool) EventName() string    { return "agentactivity.tool_completed." + c.ChatID }
func (c CompleteTool) ShouldSnapshot() bool { return false }

func (c CompleteTool) Validate(*domain.AgentActivity) error {
	if err := requireChat("complete tool", c.ChatID); err != nil {
		return err
	}
	return requireID("complete tool", "tool id", c.ToolID)
}

func (c CompleteTool) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)

	call, known := next.Tools[c.ToolID]
	if !known {
		call = domain.ActivityToolCall{
			ID:        c.ToolID,
			TurnID:    currentTurn(&next),
			ChatID:    c.ChatID,
			Seq:       next.Seq,
			Name:      c.Name,
			Target:    c.Target,
			StartedAt: c.Now,
		}
	}
	delete(next.Tools, c.ToolID)
	if len(next.Tools) == 0 {
		next.Tools = nil
	}

	if c.Name != "" {
		call.Name = c.Name
	}
	if c.Target != "" {
		call.Target = c.Target
	}
	call.ResultRef = c.ResultRef
	call.Error = c.Error
	call.Status = c.Status
	if call.Status == "" {
		call.Status = domain.ToolStatusOK
	}
	call.DurationMS = c.DurationMS

	if call.DurationMS == 0 && known {
		call.DurationMS = int(c.Now.Sub(call.StartedAt).Milliseconds())
	}
	call.EndedAt = at(c.Now)

	dropChoicesForTool(&next, c.ToolID, call.Name)

	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	}
	return next
}
