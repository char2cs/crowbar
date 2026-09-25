package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// InvokeSubagentTool and CompleteSubagentTool are InvokeTool/CompleteTool's
// twins for a tool call a SUBAGENT's own nested conversation made, not the
// chat's top-level turn. They stamp SubagentID instead of a TurnID and,
// deliberately, never call ensureTurn: a subagent's own tool call must never
// open (or attach to) a top-level turn — the chat's own turn may already be
// closed by the time a routed child frame arrives, and inventing one here
// would be exactly the bleed-through this whole mechanism exists to avoid.

type InvokeSubagentTool struct {
	ChatID     string
	SubagentID string
	ToolID     string
	Name       string
	Target     string
	RequestRef string
	Now        time.Time
}

func (c InvokeSubagentTool) AggregateID() string { return c.ChatID }
func (c InvokeSubagentTool) EventName() string {
	return "agentactivity.subagent_tool_invoked." + c.ChatID
}
func (c InvokeSubagentTool) ShouldSnapshot() bool { return false }

func (c InvokeSubagentTool) Validate(*domain.ChatActivity) error {
	if err := requireChat("invoke subagent tool", c.ChatID); err != nil {
		return err
	}
	if err := requireID("invoke subagent tool", "subagent id", c.SubagentID); err != nil {
		return err
	}
	return requireID("invoke subagent tool", "tool id", c.ToolID)
}

func (c InvokeSubagentTool) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)

	call := domain.ActivityToolCall{
		ID:         c.ToolID,
		SubagentID: c.SubagentID,
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

type CompleteSubagentTool struct {
	ChatID     string
	SubagentID string
	ToolID     string
	Name       string
	Target     string
	ResultRef  string
	Status     string
	Error      string
	DurationMS int
	Now        time.Time
}

func (c CompleteSubagentTool) AggregateID() string { return c.ChatID }
func (c CompleteSubagentTool) EventName() string {
	return "agentactivity.subagent_tool_completed." + c.ChatID
}
func (c CompleteSubagentTool) ShouldSnapshot() bool { return true }

func (c CompleteSubagentTool) Validate(*domain.ChatActivity) error {
	if err := requireChat("complete subagent tool", c.ChatID); err != nil {
		return err
	}
	if err := requireID("complete subagent tool", "subagent id", c.SubagentID); err != nil {
		return err
	}
	return requireID("complete subagent tool", "tool id", c.ToolID)
}

func (c CompleteSubagentTool) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)

	call, known := next.Tools[c.ToolID]
	if !known {
		call = domain.ActivityToolCall{
			ID:         c.ToolID,
			SubagentID: c.SubagentID,
			ChatID:     c.ChatID,
			Seq:        next.Seq,
			Name:       c.Name,
			StartedAt:  c.Now,
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
	if call.SubagentID == "" {
		call.SubagentID = c.SubagentID
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

	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	}
	return next
}
