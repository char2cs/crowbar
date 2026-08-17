// Package commands holds the AgentActivity aggregate's commands, one per file.
//
// Every command follows the same discipline: it advances the sequence, mutates
// only the open state, and records exactly what it touched in Last. Last is not a
// convenience — it is the only channel a projection has, because an event carries
// nothing but its name and the patch between two states.
//
// Snapshot policy differs by frequency. Turn boundaries snapshot; tool, subagent
// and interruption events do not. A snapshot is one upserted row and therefore
// cheap, but tool events are the highest-frequency events in the system —
// hundreds per turn — and a snapshot at each turn boundary already bounds any cold
// load to replaying a single turn's worth of deltas.
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// advance copies the aggregate, bumps its sequence and clears the previous
// delta. Every command starts here, so no command can accidentally publish the
// item its predecessor touched.
func advance(current *domain.AgentActivity, chatID string) domain.AgentActivity {
	if current == nil {
		return domain.AgentActivity{ChatID: chatID, Seq: 1}
	}
	next := *current
	next.ChatID = chatID
	next.Seq = current.Seq + 1
	next.Last = nil
	next.Tools = cloneTools(current.Tools)
	next.Subagents = cloneSubagents(current.Subagents)
	next.Interruptions = cloneInterruptions(current.Interruptions)
	return next
}

// The maps are copied because EmitEvent must be pure: asynx diffs the emitted
// value against the previous one, and a map mutated in place is the same map on
// both sides — the patch would come out empty and the change would vanish.
func cloneTools(in map[string]domain.ActivityToolCall) map[string]domain.ActivityToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivityToolCall, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSubagents(in map[string]domain.ActivitySubagent) map[string]domain.ActivitySubagent {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivitySubagent, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneInterruptions(in map[string]domain.ActivityInterruption) map[string]domain.ActivityInterruption {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivityInterruption, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func requireChat(op, chatID string) error {
	if chatID == "" {
		return fmt.Errorf("%s: missing chat id: %w", op, asynxModels.ErrValidation)
	}
	return nil
}

func requireID(op, name, id string) error {
	if id == "" {
		return fmt.Errorf("%s: missing %s: %w", op, name, asynxModels.ErrValidation)
	}
	return nil
}

// full reports whether the open-state maps have hit their ceiling. Refusing past
// it keeps a provider that opens items and never closes them from growing the
// aggregate without bound — the one property this whole shape exists to hold.
func full(a *domain.AgentActivity) bool {
	return a != nil && a.OpenCount() >= domain.MaxOpenPerTurn
}

func at(t time.Time) *time.Time {
	v := t
	return &v
}
