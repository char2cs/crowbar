package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Interrupt records the agent being blocked on, or interrupted by, something
// outside the turn — a permission prompt, a notification, a compaction.
//
// These are the events whose absence produced every legibility failure observed
// in live testing: a provider blocked on a trust prompt rendered as silence, and
// a compaction rendered as nothing at all.
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

	// An interruption that arrives with NO TURN OPEN is a moment, not a state, and
	// is recorded already resolved.
	//
	// Measured against claude 2.1.233 on 2026-08-17: a Notification saying "Claude
	// is waiting for your input" fires a MINUTE after the turn ended. That is the
	// agent being idle, not blocked — and held open it would render a permanent
	// "the agent needs your attention" banner over an agent that is perfectly
	// fine. Nothing resolves it either, because neither provider has an event
	// that ends a notification.
	//
	// Blocking is therefore defined by the thing that actually distinguishes it:
	// the agent stopped MID-TURN, with a turn still open to be blocked in.
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

// ResolveInterruption records an interruption ending.
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
