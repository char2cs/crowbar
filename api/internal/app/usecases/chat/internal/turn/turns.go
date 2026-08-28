// Package turn is the vendor CLI's hook ingress and everything it writes.
//
// A hook is a fait accompli: by the time one arrives the CLI has already acted,
// so nothing here may refuse one and nothing here may block on a lock a
// user-initiated path can hold. What it does instead is record — the turn, the
// tool call, the subagent, the interruption, the streamed message, the telemetry
// — and keep the two process-local views of "is this chat busy" honest, because
// the read model behind them is asynchronous and a switch that trusted it would
// kill a CLI mid-answer.
package turn

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Turns is the hook ingress.
type Turns struct {
	chats       agentchat.EventStore
	runnerStore agentrunner.EventStore
	activity    agentactivity.EventStore
	telemetry   *telemetry.Store
	agents      engineagents.Agents
	ws          seam.WorkspaceReader
	home        func() (string, error)
	// work is the authoritative process-local mirror of AgentChat.Working returned
	// by the turn commands. Unlike the projection, it cannot briefly report idle
	// after a hook has durably announced background work.
	work *inflight.Work
	// turns is the in-flight-turn registry a provider switch BLOCKS on, so it
	// never quits a CLI mid-answer.
	turns *inflight.Turns
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *inflight.Gate
	// messages assembles each assistant message from the increments its provider
	// streams, because the terminating hook carries only the LAST message of a turn.
	messages *stream.Streams
	// pendingHooks is the fork-before-runner-persistence barrier: hooks that arrive
	// before the runner row exists are buffered into it and replayed after.
	pendingHooks *inflight.Hooks
	// hookDeliveries durably deduplicates Crowbar relay retries before any turn
	// state or ledger mutation. The relay owns retry/spooling; this journal owns
	// the exactly-once ingress boundary.
	hookDeliveries agentjournal.HookDeliveries
	// hookGates serialises one runner's hook ingestion. It is held across the WHOLE
	// ingest — dedupe, replay buffering, effects, completion — which is why it
	// lives here rather than inside the delivery journal.
	hookGates *inflight.Gate
	// answers is the desk a provider prompt parks a blocked hook relay on.
	answers *answerdesk.Desk

	conversations Conversations
	// runners is reached for the placement half of a hook and for the prompt
	// journal a user_prompt confirms. Bound after construction: the two sides are
	// built together and neither can name the other first.
	runners Runners

	// messageDelta fans a growing assistant message out to any client watching.
	// Wired at sweep start rather than at construction, because what it publishes
	// through is the hub — a layer above this one. Nil until then, and nil forever
	// in a daemon with no detector.
	messageDelta func(chatID, workspaceID, messageID, text string)
}

// Deps is everything the hook ingress is built over. It is a struct and not an
// argument list because the shared in-flight state is built once, elsewhere, and
// handed here — an ordered list of sixteen pointers is a place to swap two of them
// silently.
type Deps struct {
	Chats     agentchat.EventStore
	Runners   agentrunner.EventStore
	Activity  agentactivity.EventStore
	Telemetry *telemetry.Store
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Home      func() (string, error)

	Work          *inflight.Work
	InflightTurns *inflight.Turns
	TurnStarts    *inflight.Gate
	PendingHooks  *inflight.Hooks
	Answers       *answerdesk.Desk

	Conversations Conversations
}

// New builds the hook ingress. The runner port is bound separately, by
// SetRunners, because the two call each other.
func New(d Deps) *Turns {
	return &Turns{
		chats:       d.Chats,
		runnerStore: d.Runners,
		activity:    d.Activity,
		telemetry:   d.Telemetry,
		agents:      d.Agents,
		ws:          d.Workspace,
		home:        d.Home,
		work:        d.Work,
		turns:       d.InflightTurns,
		turnStarts:  d.TurnStarts,
		// Owned outright, so built here rather than handed in: the message streams,
		// the exactly-once ingress journal and the per-runner ingest gate are named
		// by nothing outside this package.
		messages:       stream.New(),
		hookDeliveries: agentjournal.NewHookDeliveries(),
		hookGates:      inflight.NewGate(),
		pendingHooks:   d.PendingHooks,
		answers:        d.Answers,

		conversations: d.Conversations,
	}
}

// SetRunners binds the runner lifecycle this package reaches for the placement
// half of a hook.
func (t *Turns) SetRunners(runners Runners) { t.runners = runners }

// SetMessageDelta wires the fan-out for a growing assistant message. It is called
// at sweep start, not at construction: a daemon with nobody to publish to records
// the message when it finishes instead.
func (t *Turns) SetMessageDelta(fn func(chatID, workspaceID, messageID, text string)) {
	t.messageDelta = fn
}

// SetHookDeliveries replaces the exactly-once ingress journal. It exists for the
// deterministic durability faults the usecase's tests inject; production always
// uses the fsync-on-rename journal New builds.
//
// It REPLACES the journal, so it must be called before the runner under test has
// delivered anything: the in-memory completion markers do not survive it.
func (t *Turns) SetHookDeliveries(deliveries agentjournal.HookDeliveries) {
	t.hookDeliveries = deliveries
}

// HookDeliveryMarkers are the in-memory completion markers the ingress journal is
// holding. A marker answers a repeat delivery without ever reading the disk, so a
// test that means to exercise the on-disk record must first prove the marker is
// absent.
func (t *Turns) HookDeliveryMarkers() []string { return t.hookDeliveries.CompletionMarkers() }
