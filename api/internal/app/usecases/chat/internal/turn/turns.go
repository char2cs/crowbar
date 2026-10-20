// Package turn records what a vendor CLI's hooks report.
//
// A hook is a fait accompli: by the time one arrives the CLI has already acted.
// What this package does is record it — the turn, the tool call, the subagent,
// the interruption, the streamed message, the telemetry — and decide the turn in
// the chat's ledger, which alone says it is busy. The ingress calls it under the
// chat's lock, so nothing here waits on anything another hook delivers.
package turn

import (
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Turns records hooks.
type Turns struct {
	chats     agentchat.EventStore
	activity  agentactivity.EventStore
	telemetry *telemetry.Store
	agents    engineagents.Agents
	ws        seam.WorkspaceReader
	home      func() (string, error)
	ledger    *ledgerstore.Store
	// messages assembles each assistant message from the increments its provider
	// streams, because the terminating hook carries only the LAST message of a turn.
	messages *stream.Streams
	// live holds the streamed text that is shown while it happens and never
	// recorded — the model's thinking, a running tool's output. See livetext.go.
	live *liveText
	// compacting latches which turn id belongs to a compact_start round trip,
	// so its own turn-end-shaped close is never misread as an ordinary
	// reply or failure. See compaction.go.
	compacting *compactionTurns
	// manualCompact latches which chat's next compaction was asked for by
	// Crowbar itself, for a provider whose own wire event never says so. See
	// compaction.go.
	manualCompact *manualCompactRequests
	// answers is the desk a provider prompt parks a blocked hook relay on.
	answers *answerdesk.Desk

	conversations Conversations
	// runners is reached for the placement half of a hook. Bound after
	// construction: the two sides are built together and neither can name the
	// other first.
	runners Runners

	// feed publishes the live facts no projection carries — a growing message,
	// the compaction edge, the plan, the usage report — to any client watching.
	// Wired at sweep start rather than at construction, because what it
	// publishes through is the hub, a layer above this one. Zero (every field
	// nil) until then, and forever in a daemon with no detector wiring.
	feed seam.ChatFeed
	// choiceFeed dedups what PublishChoices pushes.
	choiceFeed *choiceFeed
}

// Deps is everything Turns is built over. It is a struct and not an argument
// list because the shared in-flight state is built once, elsewhere, and handed
// here — an ordered list of pointers is a place to swap two of them silently.
type Deps struct {
	Chats     agentchat.EventStore
	Activity  agentactivity.EventStore
	Telemetry *telemetry.Store
	Agents    engineagents.Agents
	Workspace seam.WorkspaceReader
	Home      func() (string, error)
	Ledger    *ledgerstore.Store

	Answers *answerdesk.Desk

	Conversations Conversations
}

// New builds Turns. The runner port is bound separately, by SetRunners,
// because the two call each other.
func New(d Deps) *Turns {
	return &Turns{
		chats:     d.Chats,
		activity:  d.Activity,
		telemetry: d.Telemetry,
		agents:    d.Agents,
		ws:        d.Workspace,
		home:      d.Home,
		ledger:    d.Ledger,
		// Owned outright, so built here rather than handed in: nothing outside
		// this package names the message streams.
		messages:      stream.New(),
		live:          newLiveText(),
		compacting:    newCompactionTurns(),
		manualCompact: newManualCompactRequests(),
		answers:       d.Answers,
		choiceFeed:    newChoiceFeed(),

		conversations: d.Conversations,
	}
}

// SetRunners binds the runner lifecycle this package resolves a hook's runner
// through.
func (t *Turns) SetRunners(runners Runners) { t.runners = runners }

// SetFeed wires the live chat feed. Called at sweep start: a daemon with
// nobody to publish to records the message when it finishes instead.
func (t *Turns) SetFeed(feed seam.ChatFeed) { t.feed = feed }

// ForgetChat drops every piece of in-memory turn state held for chatID: the
// chat is being erased (A7), and none of it may outlive the chat.
func (t *Turns) ForgetChat(chatID string) {
	t.live.forget(chatID)
	t.messages.ForgetChat(chatID)
	t.compacting.forget(chatID)
	t.manualCompact.consume(chatID)
}
