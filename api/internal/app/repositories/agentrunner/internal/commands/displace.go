package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Displace takes a runner OFF the chat and conversation it was on, without saying
// anything at all about whether its process is alive.
//
// Read that twice, because it is the distinction the whole model rests on. This command
// does NOT violate "the PTY is the sole authority on liveness": it asserts no liveness
// fact. The runner's row survives — it is still a running CLI, and it still dies only
// when its PTY does (Exit). What Displace states is a PLACEMENT fact, and placement is
// the one thing Crowbar solely owns: *this runner is no longer placed anywhere.*
//
// It is issued whenever Crowbar takes a CLI off a chat — an eviction (invariant I3), the
// outgoing side of a provider switch, a chat being deleted under it — and it is issued
// EVEN IF the subsequent kill fails, because "I have taken it off this chat" is true the
// moment we decide it, while "it is dead" is not ours to decide.
//
// That is what makes I2/I3 hold at every INSTANT rather than eventually:
//
//   - a SIGTERM'd CLI does not die synchronously, so without this a switch or an
//     eviction leaves two runners pointed at one chat for as long as the corpse takes to
//     fall over — and the read model has to GUESS which one is real. Guessing by "who
//     arrived last" is wrong the moment the dying CLI announces a conversation it started
//     BEFORE the switch (it stamps a newer timestamp than the incoming runner's spawn),
//     and hands out the corpse.
//   - an evicted CLI whose kill FAILS is still alive and, unplaced, can no longer write:
//     its hooks resolve to no chat and are dropped, instead of polluting the ledger of the
//     chat the mover now owns.
//
// It rejects nothing and refuses nothing — the CLI's fait accompli is still recorded
// (spec §3). It is the OPPOSITE of the old OpenSegment.Validate guard: that one refused a
// move after a destructive write; this one records a move we are making ourselves.
type Displace struct {
	RunnerID string
}

func (c Displace) AggregateID() string  { return c.RunnerID }
func (c Displace) EventName() string    { return "agentrunner.displaced." + c.RunnerID }
func (c Displace) ShouldSnapshot() bool { return false }

func (c Displace) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("displace runner: no runner: %w", asynxModels.ErrValidation)
	}
	// Deliberately lenient: displacing an already-displaced runner is an idempotent
	// no-op, never an error. Every caller is a best-effort teardown, and a teardown that
	// can fail because someone got there first is a teardown that will be skipped.
	return nil
}

// EmitEvent clears the placement. CurrentSessionSince goes with it: it describes when
// the runner took the conversation it is no longer on, and leaving it set would let a
// timestamp outlive the fact it timestamps.
func (c Displace) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentChatID = ""
	next.CurrentSession = ""
	next.CurrentSessionSince = time.Time{}
	return next
}
