package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type StartSubagent struct {
	ChatID     string
	SubagentID string
	AgentType  string
	Now        time.Time
}

func (c StartSubagent) AggregateID() string  { return c.ChatID }
func (c StartSubagent) EventName() string    { return "agentactivity.subagent_started." + c.ChatID }
func (c StartSubagent) ShouldSnapshot() bool { return false }

func (c StartSubagent) Validate(*domain.ChatActivity) error {
	if err := requireChat("start subagent", c.ChatID); err != nil {
		return err
	}
	return requireID("start subagent", "subagent id", c.SubagentID)
}

func (c StartSubagent) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	sub := domain.ActivitySubagent{
		ID:        c.SubagentID,
		TurnID:    ensureTurn(&next, c.Now),
		ChatID:    c.ChatID,
		Seq:       next.Seq,
		AgentType: c.AgentType,
		StartedAt: c.Now,
	}
	if !full(&next) {
		if next.Subagents == nil {
			next.Subagents = map[string]domain.ActivitySubagent{}
		}
		next.Subagents[c.SubagentID] = sub
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaSubagent, Subagent: &sub,
	}
	return next
}

// OpenNestedSubagent is StartSubagent's twin for a codex-shaped provider's
// own multi-agent tool call — opened from a completed TOOL CALL naming a
// nested session id (see turn/observation.go's openNestedSubagent), not a
// dedicated subagent_pre hook. It deliberately never calls ensureTurn the
// way StartSubagent does: the chat's own top-level turn may already be
// closed by the time this fires (codex ends its OWN turn the instant it
// delegates — see turn/ingest.go's namesAnotherConversation doc), and
// opening one here would silently mint a ghost top-level turn nothing ever
// really ran — exactly the bleed-through this whole mechanism exists to
// avoid. TurnID stays empty, the same convention a nested tool call's own
// TurnID already follows (see commands.InvokeSubagentTool).
type OpenNestedSubagent struct {
	ChatID     string
	SubagentID string
	Now        time.Time
}

func (c OpenNestedSubagent) AggregateID() string { return c.ChatID }
func (c OpenNestedSubagent) EventName() string {
	return "agentactivity.nested_subagent_opened." + c.ChatID
}
func (c OpenNestedSubagent) ShouldSnapshot() bool { return false }

func (c OpenNestedSubagent) Validate(*domain.ChatActivity) error {
	if err := requireChat("open nested subagent", c.ChatID); err != nil {
		return err
	}
	return requireID("open nested subagent", "subagent id", c.SubagentID)
}

func (c OpenNestedSubagent) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	sub := domain.ActivitySubagent{
		ID: c.SubagentID, ChatID: c.ChatID, Seq: next.Seq, StartedAt: c.Now,
	}
	if !full(&next) {
		if next.Subagents == nil {
			next.Subagents = map[string]domain.ActivitySubagent{}
		}
		next.Subagents[c.SubagentID] = sub
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaSubagent, Subagent: &sub,
	}
	return next
}

type StopSubagent struct {
	ChatID     string
	SubagentID string
	AgentType  string
	// Message is the subagent's own final reply text, when this close carries
	// one — a routed child turn's own close, for a provider whose subagent is
	// a whole nested conversation, not a flat marker. Empty appends nothing.
	Message string
	Now     time.Time
}

func (c StopSubagent) AggregateID() string  { return c.ChatID }
func (c StopSubagent) EventName() string    { return "agentactivity.subagent_stopped." + c.ChatID }
func (c StopSubagent) ShouldSnapshot() bool { return false }

func (c StopSubagent) Validate(*domain.ChatActivity) error {
	if err := requireChat("stop subagent", c.ChatID); err != nil {
		return err
	}
	return requireID("stop subagent", "subagent id", c.SubagentID)
}

func (c StopSubagent) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	sub, known := next.Subagents[c.SubagentID]
	if !known {
		sub = domain.ActivitySubagent{
			ID:        c.SubagentID,
			TurnID:    currentTurn(&next),
			ChatID:    c.ChatID,
			Seq:       next.Seq,
			AgentType: c.AgentType,
			StartedAt: c.Now,
		}
	}
	delete(next.Subagents, c.SubagentID)
	if len(next.Subagents) == 0 {
		next.Subagents = nil
	}
	if c.AgentType != "" {
		sub.AgentType = c.AgentType
	}
	if c.Message != "" {
		sub.Messages = append(sub.Messages, domain.ActivitySubagentMessage{
			Text: c.Message, At: c.Now,
		})
	}
	sub.EndedAt = at(c.Now)
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaSubagent, Subagent: &sub,
	}
	return next
}
