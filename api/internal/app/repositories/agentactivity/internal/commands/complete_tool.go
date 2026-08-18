package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CompleteTool records a tool call finishing.
//
// A completion for a call the aggregate never saw is recorded anyway, using what
// the completion itself carries. A provider that reports only the post hook — or
// whose pre hook was lost when the daemon restarted mid-turn — should still leave
// a legible record, and a dropped completion would show a tool as running forever.
//
// A FAILING tool arrives here too, and must: claude fires PostToolUseFailure
// INSTEAD OF PostToolUse (measured against 2.1.234 on 2026-08-17), so a failure
// that did not complete the call left it in flight until the turn-close sweep
// abandoned it — "the Edit failed" rendered as "the Edit is still running".
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

	// The completion is authoritative only for what it actually reports: a post
	// hook that omits the tool name must not erase the name the pre hook gave us.
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
	// Prefer the provider's own duration; fall back to what Crowbar observed, and
	// only when a start was actually seen.
	if call.DurationMS == 0 && known {
		call.DurationMS = int(c.Now.Sub(call.StartedAt).Milliseconds())
	}
	call.EndedAt = at(c.Now)

	// A prompt gating this call is answered the moment the call proceeds. Nobody
	// reports that answer — it is typed at the PTY — so the work moving on is the
	// only evidence there is, and waiting for better evidence leaves a question
	// pinned over a chat that has long since gone on without it.
	dropChoicesForTool(&next, c.ToolID, call.Name)

	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	}
	return next
}
