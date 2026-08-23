package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type Interrupt struct {
	ChatID string
	ID     string
	Kind   string
	Detail string
	Now    time.Time
}

func (c Interrupt) AggregateID() string  { return c.ChatID }
func (c Interrupt) EventName() string    { return "agentactivity.interrupted." + c.ChatID }
func (c Interrupt) ShouldSnapshot() bool { return false }

func (c Interrupt) Validate(*domain.AgentActivity) error {
	if err := requireChat("interrupt", c.ChatID); err != nil {
		return err
	}
	if err := requireID("interrupt", "interruption id", c.ID); err != nil {
		return err
	}
	return requireID("interrupt", "kind", c.Kind)
}

func (c Interrupt) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)

	idle := next.Turn == nil
	item := domain.ActivityInterruption{
		ID:     c.ID,
		ChatID: c.ChatID,
		Seq:    next.Seq,
		Kind:   c.Kind,
		Detail: c.Detail,
		At:     c.Now,
	}
	if idle {
		item.ResolvedAt = at(c.Now)
		next.Last = &domain.ActivityDelta{
			Phase: domain.DeltaClose, Kind: domain.DeltaInterruption, Interruption: &item,
		}
		return next
	}

	item.TurnID = next.Turn.ID
	if !full(&next) {
		if next.Interruptions == nil {
			next.Interruptions = map[string]domain.ActivityInterruption{}
		}
		next.Interruptions[c.ID] = item
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaInterruption, Interruption: &item,
	}
	return next
}

type ResolveInterruption struct {
	ChatID string
	ID     string
	Kind   string
	Detail string
	Now    time.Time
}

func (c ResolveInterruption) AggregateID() string { return c.ChatID }

func (c ResolveInterruption) EventName() string {
	return "agentactivity.interruption_resolved." + c.ChatID
}
func (c ResolveInterruption) ShouldSnapshot() bool { return false }

func (c ResolveInterruption) Validate(*domain.AgentActivity) error {
	if err := requireChat("resolve interruption", c.ChatID); err != nil {
		return err
	}
	return requireID("resolve interruption", "interruption id", c.ID)
}

func (c ResolveInterruption) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	item, known := next.Interruptions[c.ID]
	if !known {
		item = domain.ActivityInterruption{
			ID:     c.ID,
			TurnID: currentTurn(&next),
			ChatID: c.ChatID,
			Seq:    next.Seq,
			Kind:   c.Kind,
			Detail: c.Detail,
			At:     c.Now,
		}
	}
	delete(next.Interruptions, c.ID)
	if len(next.Interruptions) == 0 {
		next.Interruptions = nil
	}
	if c.Detail != "" {
		item.Detail = c.Detail
	}
	item.ResolvedAt = at(c.Now)
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaInterruption, Interruption: &item,
	}
	return next
}
