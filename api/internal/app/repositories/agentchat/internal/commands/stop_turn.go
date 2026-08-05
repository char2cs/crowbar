package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// StopTurn closes an agent turn and records the level of asynchronous work the CLI
// reported STILL OUTSTANDING as it went quiet. The chat stops working only if that
// level is zero: a turn that ended because the CLI handed work to a background task
// keeps the chat working through the wait.
//
// AsyncWork is the CLI's OWN restated number, carried on this very turn_stop, not a
// tally Crowbar maintains across events. Restating it here — every time, wholesale —
// is what makes the spinner impossible to strand: there is no accumulator to drift,
// and the last word always belongs to the most recent report.
//
// Abandoned forces the level to zero and belongs to the reconcile paths ONLY (a dead
// CLI, a displaced runner). It is the difference between the two reasons a turn ends.
// An ordinary turn_stop hook says "I am done talking for now" and reports what it left
// running — it must not clear that, or the spinner darkens under a live subagent, which
// is the bug. A reconcile says the PROCESS IS GONE, and work announced by a CLI cannot
// outlive that CLI: nothing will ever restate the level, so whatever it last said would
// stand forever — and this is an event-sourced aggregate, so "forever" survives a
// restart. Zeroing there asserts nothing new; it is the same "reconciled, not
// authoritative" repair the turn itself already gets.
type StopTurn struct {
	ChatID string
	Now    time.Time
	// AsyncWork is the level reported by THIS turn_stop. Ignored when Abandoned.
	AsyncWork int
	Abandoned bool
}

func (c StopTurn) AggregateID() string  { return c.ChatID }
func (c StopTurn) EventName() string    { return "agentchat.turn_stopped." + c.ChatID }
func (c StopTurn) ShouldSnapshot() bool { return false }

// Validate refuses a chat that does not exist and — for an ABANDON only — a chat that
// has nothing to close.
//
// That second refusal is where the "is there a turn here?" decision lives, and it lives
// here because here is the only place it can be asked without a race. asynx loads the
// aggregate and its version together and appends the event at that same version, so a
// condition evaluated in Validate is evaluated against the authoritative fold of the
// event log and committed atomically with it: a turn opening in between collides on the
// version and the caller's OCC retry re-validates against the new state.
//
// It used to be asked by the caller instead (agent.closeAbandonedTurn read
// domain.AgentChat.Working off the READ MODEL and returned early when it said idle) —
// and the read model is folded by an ASYNCHRONOUS projection, so a turn that was already
// durable in the log could still read as idle. The caller took the early return, nothing
// ever closed the turn, and the chat spun forever. A reconcile must not decide on
// projected state.
//
// An ordinary turn_stop is NOT held to this: the hook is the CLI restating its async-work
// level, and a level arriving on a chat the read model calls idle is exactly the report
// that must be recorded.
func (c StopTurn) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("stop turn: no chat: %w", asynxModels.ErrValidation)
	}
	if c.Abandoned && !foldWorking(current) {
		return fmt.Errorf("stop turn: nothing to abandon: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c StopTurn) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.AsyncWork = c.AsyncWork
	if c.Abandoned {
		next.AsyncWork = 0
	}
	// A negative level is not a fact any CLI can report; it could only come from a
	// misconfigured descriptor path. Floor it rather than let it stand, where it
	// would read as idle just the same but corrupt the next comparison.
	if next.AsyncWork < 0 {
		next.AsyncWork = 0
	}
	next.CurrentTurnStarted = nil
	next.Working = foldWorking(&next)
	next.LastActivityAt = c.Now
	return next
}
