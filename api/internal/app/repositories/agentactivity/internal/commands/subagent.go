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

func (c StartSubagent) Validate(*domain.AgentActivity) error {
	if err := requireChat("start subagent", c.ChatID); err != nil {
		return err
	}
	return requireID("start subagent", "subagent id", c.SubagentID)
}

func (c StartSubagent) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
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

type StopSubagent struct {
	ChatID     string
	SubagentID string
	AgentType  string
	Now        time.Time
}

func (c StopSubagent) AggregateID() string  { return c.ChatID }
func (c StopSubagent) EventName() string    { return "agentactivity.subagent_stopped." + c.ChatID }
func (c StopSubagent) ShouldSnapshot() bool { return false }

func (c StopSubagent) Validate(*domain.AgentActivity) error {
	if err := requireChat("stop subagent", c.ChatID); err != nil {
		return err
	}
	return requireID("stop subagent", "subagent id", c.SubagentID)
}

func (c StopSubagent) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
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
	sub.EndedAt = at(c.Now)
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaSubagent, Subagent: &sub,
	}
	return next
}
