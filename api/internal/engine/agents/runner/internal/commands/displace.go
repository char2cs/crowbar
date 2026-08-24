package commands

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
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
// It is what BOUNDS the window in which I2/I3 are violated — and the honest word is
// "bounds", not "eliminates". Read the two cases, because they differ and the difference
// is load-bearing:
//
//   - PROVIDER SWITCH / CHAT DELETE: the displace happens BEFORE the incoming CLI is
//     spawned, so there is never an instant where two runners are placed on the chat. A
//     SIGTERM'd CLI does not die synchronously, and without this the corpse would stay
//     pointed at the chat for as long as it took to fall over — with the read model left to
//     GUESS which of the two is real. Guessing by "who arrived last" is wrong the moment the
//     dying CLI announces a conversation it opened BEFORE the switch (it stamps a newer
//     timestamp than the incoming runner's spawn), and hands out the corpse.
//
//   - EVICTION: the mover's Move is recorded FIRST — reality is not negotiable (spec §3) —
//     and the incumbent is displaced immediately after. So an eviction genuinely TRANSITS a
//     two-candidate window, and the read model's newest-arrival ordering is what resolves it
//     correctly (the mover just took the conversation, so it is the newest arrival). The
//     ordering is not decoration there; do not delete it as dead code.
//
// And whatever happens to the kill, the displacement stands: an evicted CLI whose kill
// FAILS is still alive but unplaced, so its hooks resolve to no chat and are dropped,
// instead of polluting the ledger of the chat the mover now owns.
//
// It rejects nothing and refuses nothing — the CLI's fait accompli is still recorded
// (spec §3). It is the OPPOSITE of the old OpenSegment.Validate guard: that one refused a
// move after a destructive write; this one records a move we are making ourselves.
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

// EmitEvent clears the placement. CurrentSessionSince goes with it: it describes when
// the runner took the conversation it is no longer on, and leaving it set would let a
// timestamp outlive the fact it timestamps.
func (c Displace) EmitEvent(current *agents.Runner) agents.Runner {
	next := *current
	next.CurrentChatID = ""
	next.CurrentSession = ""
	next.CurrentSessionSince = time.Time{}
	next.CurrentSessionResumable = false
	return next
}
