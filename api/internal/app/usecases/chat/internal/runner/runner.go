// Package runner is the vendor CLI's lifecycle: starting one on a chat,
// replacing it with another provider's, resuming it after a restart, stopping
// it, and delivering a Crowbar-authored prompt into it.
//
// Everything here is USER-INITIATED, which is what separates it from the hook
// ingress. It may block, it may refuse, and it takes the per-chat spawn gate —
// the only thing that stops two clicks putting two CLIs on one chat. That is
// also why nothing on the hook path may call into it through a gated door: a
// switch holds the gate while parked on a turn only a hook can end.
//
// The PTY is the sole authority on liveness. Nothing here asserts a process is
// alive or dead that it has not observed; a CLI is ended by killing it and
// letting its death carry the runner row away.
package runner

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Runners is the CLI lifecycle.
type Runners struct {
	chats       agentchat.EventStore
	runnerStore agentrunner.EventStore
	activity    agentactivity.EventStore
	agents      engineagents.Agents
	term        seam.TerminalCommander
	ws          seam.WorkspaceReader
	// spawns serialises the USER-INITIATED spawn paths per chat. It is the only
	// thing that can stop two concurrent switches putting two CLIs on one chat, and
	// it is NEVER taken on the hook path.
	spawns *inflight.Gate
	// inflightTurns is the in-flight-turn registry this type BLOCKS on before it
	// quits a CLI.
	inflightTurns *inflight.Turns
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *inflight.Gate
	work       *inflight.Work
	// prompts is the durable at-most-once React-submission journal and the
	// process-local transition lock shared with user_prompt hook confirmation.
	prompts agentjournal.PromptRequests
	// pendingHooks is the fork-before-runner-persistence barrier this type installs
	// before the fork and finishes after the runner row exists.
	pendingHooks *inflight.Hooks
	// catalogs owns only cancellation for in-flight deterministic probes. Results
	// are deliberately never cached.
	catalogs *catalog.Runs
	// minter issues the per-runner token an MCP call is authenticated by.
	minter *agenttools.TokenMinter
	// answers is the desk a dead runner's blocked relays are released from.
	answers *answerdesk.Desk
	// apiConns holds each runner's api-transport connection (serve process +
	// driver), for a mixed-transport provider. Empty for a hooks-only one.
	apiConns *apiConnRegistry
	// attached holds, for a runner CURRENTLY showing its provider's native TUI
	// instead of its api connection, the terminal session that IS that TUI —
	// in memory only, exactly like apiConns: it describes a live process, not
	// a fact to survive a restart on. See attach.go.
	attached *attachRegistry

	conversations Conversations
	providers     Providers
	// turns is the hook ingress. Bound after construction: the two sides are built
	// together and neither can name the other first.
	turns Turns

	// termWait detects the state no hook reports: a CLI parked on a modal Crowbar
	// cannot answer, which otherwise renders as an empty pane over a live process.
	// NIL when the terminal seam cannot render a screen, in which case every chat
	// reports the zero verdict.
	termWait termwait.Detector
	// promptSettled fans out the edge where a delivery is retired without ever
	// having produced a turn. Wired at sweep start rather than at construction,
	// because what it publishes through is the hub — a layer above this one.
	promptSettled func(chatID, workspaceID, requestID string)
}

// Deps is everything the CLI lifecycle is built over. It is a struct and not an
// argument list for the same reason turn.Deps is: the shared in-flight state is
// built once, elsewhere, and an ordered list of seventeen pointers is a place to
// swap two of them silently.
type Deps struct {
	Chats     agentchat.EventStore
	Runners   agentrunner.EventStore
	Activity  agentactivity.EventStore
	Agents    engineagents.Agents
	Terminal  seam.TerminalCommander
	Workspace seam.WorkspaceReader

	Spawns        *inflight.Gate
	InflightTurns *inflight.Turns
	TurnStarts    *inflight.Gate
	Work          *inflight.Work
	PendingHooks  *inflight.Hooks
	Minter        *agenttools.TokenMinter
	Answers       *answerdesk.Desk

	Conversations Conversations
	Providers     Providers
}

// New builds the CLI lifecycle. The hook-ingress port is bound separately, by
// SetTurns, because the two call each other.
func New(d Deps) *Runners {
	return &Runners{
		chats:         d.Chats,
		runnerStore:   d.Runners,
		activity:      d.Activity,
		agents:        d.Agents,
		term:          d.Terminal,
		ws:            d.Workspace,
		spawns:        d.Spawns,
		inflightTurns: d.InflightTurns,
		turnStarts:    d.TurnStarts,
		work:          d.Work,
		// Owned outright, so built here rather than handed in: the at-most-once
		// submission journal and the probe limiter are named by nothing outside this
		// package.
		prompts:       agentjournal.NewPromptRequests(),
		catalogs:      catalog.New(),
		pendingHooks:  d.PendingHooks,
		minter:        d.Minter,
		answers:       d.Answers,
		apiConns:      newAPIConnRegistry(),
		attached:      newAttachRegistry(),
		conversations: d.Conversations,
		providers:     d.Providers,
	}
}

// SetTurns binds the hook ingress and, with it, builds the terminal-wait
// detector.
//
// The detector is built HERE rather than in New because its ports are spread
// across both sides — the screen classifiers and the message stream belong to the
// hook ingress — and it binds them by value. Building it in New would bind nil.
//
// It is nil when the terminal seam cannot render a screen, which is the whole of
// the "no detector" case every reader of termWait guards for.
func (rs *Runners) SetTurns(turns Turns) {
	rs.turns = turns
	screens, ok := rs.term.(termwait.Screens)
	if !ok {
		return
	}
	rs.termWait = termwait.New(termwait.Deps{
		Runners:    rs.runnerStore,
		Chats:      rs.chats,
		Choices:    rs.activity,
		Screens:    screens,
		Prompts:    turns,
		Notices:    turns,
		Work:       turns,
		OnStall:    turns.CloseStalledTurn,
		Deliveries: rs,
		Messages:   turns,
	})
}

// composeContext joins the non-empty sections of a spawn's {context} in the order
// given. Empty sections are dropped rather than left as blank paragraphs: a chat
// with no lineage and no handoff must not open with two empty lines of nothing.
func composeContext(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ComposeContext is composeContext, exposed for the chat usecase's own tests.
//
// The rule is reached directly rather than through a spawn because one of its
// inputs comes from the process-wide config singleton, and config.Get MEMOISES on
// first use — so a spawn-level test could not present a blanked
// capabilities_instruction without depending on which test in the binary ran
// first. A composition rule tested through a memoised global is a test that passes
// for the wrong reason.
var ComposeContext = composeContext

// SetPromptJournal replaces the at-most-once submission journal. It exists for
// the deterministic durability faults the usecase's tests inject; production
// always uses the fsync-on-rename journal New builds.
//
// It REPLACES the journal, so it must be called before the chat under test has
// submitted anything. The prompt journal holds no state between calls, so a
// replacement loses nothing.
func (rs *Runners) SetPromptJournal(prompts agentjournal.PromptRequests) { rs.prompts = prompts }
