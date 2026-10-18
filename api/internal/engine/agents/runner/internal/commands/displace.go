package commands

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

// Displace takes a runner OFF the chat it was on, without saying anything about
// whether its process is alive: the row survives until its PTY dies (Exit). It is
// a PLACEMENT fact, the one thing Crowbar solely owns, issued whenever Crowbar takes
// a CLI off a chat — the outgoing side of a provider switch, a chat deleted under
// it — before the successor is spawned and even if the kill then fails, so a dying
// CLI's hooks resolve to no chat and are dropped.
type Displace struct {
	RunnerID string
}

func (c Displace) AggregateID() string  { return c.RunnerID }
func (c Displace) EventName() string    { return "runner.displaced." + c.RunnerID }
func (c Displace) ShouldSnapshot() bool { return false }

func (c Displace) Validate(current *agents.Runner) error {
	if current == nil {
		return fmt.Errorf("displace runner: no runner: %w", asynxModels.ErrValidation)
	}
	// An EXITED runner is already nowhere: its exit cleared every placement this command
	// would have. Displacing it anyway would emit a `displaced` frame AFTER its `exited`
	// one, telling a client to let go of a runner it has already dropped. A CLI quitting on
	// its own moments before Crowbar went to take it off a chat is an ordinary, benign race,
	// so callers treat this ErrValidation as success — it is a statement that there was
	// nothing left to do, not that anything went wrong.
	if current.ExitedAt != nil {
		return fmt.Errorf("displace runner: already exited: %w", asynxModels.ErrValidation)
	}
	// Otherwise deliberately lenient: displacing an already-DISPLACED runner is an
	// idempotent no-op. Every caller is a teardown, and a teardown that can fail because
	// someone got there first is a teardown that will be skipped.
	return nil
}

// EmitEvent clears the placement.
func (c Displace) EmitEvent(current *agents.Runner) agents.Runner {
	next := *current
	next.CurrentChatID = ""
	return next
}
