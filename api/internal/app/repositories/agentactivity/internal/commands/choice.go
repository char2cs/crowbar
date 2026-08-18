package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenChoice records the agent asking a human to decide something — may I run
// this tool, which of these do you want, answer this MCP server's form.
//
// It is the one piece of open state a USER can act on, which is why it is a
// record of its own rather than another interruption: an interruption says the
// agent stopped, a choice says what it is waiting to be told.
type OpenChoice struct {
	ChatID   string
	ChoiceID string
	Kind     string
	PromptID string
	ToolName string
	Title    string
	Question string
	Mode     string
	Multi    bool
	Options  []domain.ActivityChoiceOption
	Schema   string
	Now      time.Time
}

func (c OpenChoice) AggregateID() string  { return c.ChatID }
func (c OpenChoice) EventName() string    { return "agentactivity.choice_opened." + c.ChatID }
func (c OpenChoice) ShouldSnapshot() bool { return false }

func (c OpenChoice) Validate(*domain.AgentActivity) error {
	if err := requireChat("open choice", c.ChatID); err != nil {
		return err
	}
	if err := requireID("open choice", "choice id", c.ChoiceID); err != nil {
		return err
	}
	return requireID("open choice", "kind", c.Kind)
}

func (c OpenChoice) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)

	item := domain.ActivityChoice{
		ID:       c.ChoiceID,
		ChatID:   c.ChatID,
		Seq:      next.Seq,
		Kind:     c.Kind,
		PromptID: c.PromptID,
		ToolName: c.ToolName,
		Title:    c.Title,
		Question: c.Question,
		Mode:     c.Mode,
		Multi:    c.Multi,
		Options:  c.Options,
		Schema:   c.Schema,
		At:       c.Now,
	}

	// A prompt that arrives with NO TURN OPEN is a moment, not a state, and is
	// recorded already resolved — the same rule the interruption that accompanies
	// it has followed since claude was measured firing "Claude is waiting for your
	// input" a minute AFTER a turn ended. An agent that is not running is not
	// waiting on anybody, and a pending prompt over an idle agent is a banner that
	// nothing will ever clear.
	if next.Turn == nil {
		item.ResolvedAt = at(c.Now)
		item.Resolution = domain.ChoiceResolutionAbandoned
		next.Last = &domain.ActivityDelta{
			Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
		}
		return next
	}

	item.TurnID = next.Turn.ID
	item.ToolID = gatedTool(&next, c.ToolName)
	if !full(&next) {
		if next.Choices == nil {
			next.Choices = map[string]domain.ActivityChoice{}
		}
		next.Choices[c.ChoiceID] = item
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}

// gatedTool names the in-flight tool call a permission prompt is asking about.
//
// The permission itself cannot say: measured against claude 2.1.234 on
// 2026-08-17, a PermissionRequest carries NO tool_use_id, despite claude's own
// documentation claiming one. What it does carry is the tool's NAME, and the
// PreToolUse that opened the call — which does carry the id — has already landed.
// The newest in-flight call of that name is therefore the one being gated.
//
// An empty answer is the honest one for a prompt with no tool behind it, and it
// costs nothing: resolution falls back to matching on the name.
func gatedTool(a *domain.AgentActivity, toolName string) string {
	if toolName == "" {
		return ""
	}
	found := ""
	var newest int64
	for id, call := range a.Tools {
		if call.Name != toolName || call.Seq < newest {
			continue
		}
		found, newest = id, call.Seq
	}
	return found
}

// dropChoicesForTool takes every prompt gating a tool call that has just
// finished out of the open state.
//
// This is the path that actually fires in production. A permission is almost
// always answered AT THE PTY, by a human typing into the vendor CLI, and no
// provider reports that happening: the only observable consequence is that the
// gated work proceeds. So the completion of the work IS the resolution, and a
// prompt that waited for an explicit one would hang over a chat that moved on
// minutes ago.
//
// Only the open state is touched here. The durable ROW is resolved by the
// projection, which sweeps by the same identity off the tool's own close delta —
// a delta carries one item, and that item is the tool.
func dropChoicesForTool(a *domain.AgentActivity, toolID, toolName string) {
	for id, choice := range a.Choices {
		if gatesTool(choice, toolID, toolName) {
			delete(a.Choices, id)
		}
	}
	if len(a.Choices) == 0 {
		a.Choices = nil
	}
}

// gatesTool reports whether a prompt is about this tool call. It matches on the
// adopted call id first and falls back to the name, because a permission that
// arrived before its PreToolUse — or after the call had already been swept — has
// only the name to go on.
func gatesTool(choice domain.ActivityChoice, toolID, toolName string) bool {
	if choice.ToolID != "" {
		return choice.ToolID == toolID
	}
	return toolName != "" && choice.ToolName == toolName
}

// ResolveChoice records a prompt no longer waiting on anybody.
//
// It is the EXPLICIT channel, and it is where an answer will attach: a command
// that records which options a user picked sets this same resolution to
// domain.ChoiceResolutionAnswered and needs nothing else from this shape.
type ResolveChoice struct {
	ChatID     string
	ChoiceID   string
	Resolution string
	Now        time.Time
}

func (c ResolveChoice) AggregateID() string  { return c.ChatID }
func (c ResolveChoice) EventName() string    { return "agentactivity.choice_resolved." + c.ChatID }
func (c ResolveChoice) ShouldSnapshot() bool { return false }

func (c ResolveChoice) Validate(*domain.AgentActivity) error {
	if err := requireChat("resolve choice", c.ChatID); err != nil {
		return err
	}
	return requireID("resolve choice", "choice id", c.ChoiceID)
}

func (c ResolveChoice) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	item, known := next.Choices[c.ChoiceID]
	if !known {
		// Nothing is open under that id, so there is nothing to say. The event is
		// still emitted — a resolution that was attempted is worth having in the log
		// — but it carries NO delta, because a fabricated record here would be
		// projected over a real row and blank the question it was asking.
		return next
	}
	delete(next.Choices, c.ChoiceID)
	if len(next.Choices) == 0 {
		next.Choices = nil
	}
	item.ResolvedAt = at(c.Now)
	item.Resolution = c.Resolution
	if item.Resolution == "" {
		item.Resolution = domain.ChoiceResolutionAnswered
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}
