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
	"time"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/dedup"
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
	// live holds the streamed text that is shown while it happens and never
	// recorded — the model's thinking, a running tool's output. See livetext.go.
	live *liveText
	// idle latches a provider's own "I am doing nothing" report. It is never
	// acted on directly — see idle.go.
	idle *idleLatch
	// compacting latches which turn id belongs to a compact_start round trip,
	// so its own turn/completed-shaped close is never misread as an ordinary
	// reply or failure. See compaction.go.
	compacting *compactionTurns
	// manualCompact latches which chat's next compaction was asked for by
	// Crowbar itself, for a provider whose own wire event never says so. See
	// compaction.go.
	manualCompact *manualCompactRequests
	// pendingHooks is the fork-before-runner-persistence barrier: hooks that arrive
	// before the runner row exists are buffered into it and replayed after.
	pendingHooks *inflight.Hooks
	// hookDeliveries absorbs Crowbar relay retries before any turn state or
	// ledger mutation: a bounded, in-memory TTL set of completed delivery ids.
	// The relay's retry is short and in-process (cmd/crowbar/hook_delivery.go),
	// so nothing here needs to outlive the daemon — and nothing here fsyncs.
	hookDeliveries *dedup.Set
	// hookGates serialises one runner's hook ingestion. It is held across the WHOLE
	// ingest — dedupe, replay buffering, effects, completion.
	hookGates *inflight.Gate
	// answers is the desk a provider prompt parks a blocked hook relay on.
	answers *answerdesk.Desk

	conversations Conversations
	// runners is reached for the placement half of a hook and for the prompt
	// journal a user_prompt confirms. Bound after construction: the two sides are
	// built together and neither can name the other first.
	runners Runners

	// feed publishes the live facts no projection carries — a growing message,
	// the compaction edge, the plan, the usage report — to any client watching.
	// Wired at sweep start rather than at construction, because what it
	// publishes through is the hub, a layer above this one. Zero (every field
	// nil) until then, and forever in a daemon with no detector wiring.
	feed seam.ChatFeed

	// messageAwaitTimeout bounds how long closeAssistantTurn will wait on
	// stream.Streams.AwaitOpen before concluding nothing streamed. It is a
	// self-releasing channel wait, not a lock another path can hold — it never
	// risks the deadlock the package doc warns against.
	messageAwaitTimeout time.Duration
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

// defaultMessageAwaitTimeout bounds closeAssistantTurn's wait for a message
// that may still be streaming when its turn's own closing hook arrives. Each
// hook is its own CLI-spawned subprocess, so the real gap being covered is not
// network jitter but subprocess scheduling latency under load — measured live
// exceeding 500ms with just two chats running concurrently (the hook handler
// itself blocked ~530ms, timed out, and the real delta still landed moments
// later under its own id). 3s gives real headroom against that, and this path
// is only ever reached on the rare turn actually racing — never on the
// ordinary one, which already finds its message Open and returns immediately.
const defaultMessageAwaitTimeout = 3 * time.Second

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
		// the delivery dedup set and the per-runner ingest gate are named by
		// nothing outside this package.
		messages:            stream.New(),
		live:                newLiveText(),
		idle:                newIdleLatch(),
		compacting:          newCompactionTurns(),
		manualCompact:       newManualCompactRequests(),
		hookDeliveries:      dedup.New(dedup.DefaultTTL, dedup.DefaultMax, nil),
		hookGates:           inflight.NewGate(),
		pendingHooks:        d.PendingHooks,
		answers:             d.Answers,
		messageAwaitTimeout: defaultMessageAwaitTimeout,

		conversations: d.Conversations,
	}
}

// SetRunners binds the runner lifecycle this package reaches for the placement
// half of a hook.
func (t *Turns) SetRunners(runners Runners) { t.runners = runners }

// SetMessageAwaitTimeout overrides how long closeAssistantTurn waits for a
// still-streaming message before concluding nothing streamed. Test-only
// surface: production always uses defaultMessageAwaitTimeout.
func (t *Turns) SetMessageAwaitTimeout(d time.Duration) { t.messageAwaitTimeout = d }

// SetFeed wires the live chat feed. Called at sweep start: a daemon with
// nobody to publish to records the message when it finishes instead.
func (t *Turns) SetFeed(feed seam.ChatFeed) { t.feed = feed }

// HookDeliveryCount is how many completed delivery ids the dedup set holds.
func (t *Turns) HookDeliveryCount() int { return t.hookDeliveries.Len() }
