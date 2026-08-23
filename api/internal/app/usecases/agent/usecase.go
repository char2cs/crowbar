package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type TerminalCommander interface {
	CreateCommand(
		ctx context.Context,
		workspaceID string,
		cwd string,
		argv []string,
		env []string,
		onExit func(),
	) (string, error)
	// TerminateGraceful quits a vendor CLI: a clean-exit SIGTERM, never SIGKILL.
	// It is the ONLY way this package ends a runner — we kill the process and let
	// its death carry the runner away (the engine's onExit → agentrunner.Exit →
	// the live row disappears), because the PTY is the sole authority on liveness
	// and asserting an Exit we have not observed would make us a second one.
	//
	// Graceful matters twice over: a well-behaved CLI flushes its native transcript
	// on SIGTERM, so neither the provider being switched out nor the runner being
	// EVICTED from a conversation loses its last turn — and an evicted runner's
	// conversation is about to be read by the runner taking it over. Applies
	// uniformly to every provider (Codex tolerates it too); no provider branching.
	TerminateGraceful(
		ctx context.Context,
		sessionID string,
	) error
	// SessionLive reports whether a terminal session id is backed by a LIVE PTY right
	// now. Its one caller is ReconcileRunnersOnBoot (below): it is the single authority
	// boot reconciliation asks, Exiting every runner whose PTY did not survive the
	// restart. A false here means the CLI is definitively gone, even though no event
	// ever recorded it.
	//
	// It is deliberately NOT the engine's SessionExists, which is also true for a
	// PTY-less suspended placeholder — a session whose process is already dead and
	// whose only remaining substance is scrollback on disk. Asking the registry "do
	// you know this id?" instead of "is this process alive?" is what previously let a
	// restart-orphaned chat keep advertising a live agent.
	SessionLive(
		ctx context.Context,
		sessionID string,
	) bool
}
type Usecase struct {
	chats   agentchat.EventStore
	runners agentrunner.EventStore
	// activity is the conversation record: turns, tool calls, subagents and
	// interruptions. It replaced a flat-file ledger that could hold none of the
	// last three, which is why the observation surface was impossible before it.
	activity agentactivity.EventStore
	// telemetry is the newest provider report per chat, held in memory because it
	// describes a LIVE process: a report that outlived its CLI would be a
	// confident stale number.
	telemetry *telemetryStore
	agents    engineagents.Agents
	term      TerminalCommander
	ws        WorkspaceReader
	// lineage answers "what does this chat read" at spawn time. See ChatLineage
	// and threadContext.
	lineage ChatLineage
	// providerPrefs is the global (per user/machine) priority+enabled table read by
	// ResolveProviders and rewritten by ReplaceProviderPreferences. It is keyed by
	// provider id; a provider with no row is enabled and ordered after every
	// preferenced one by descriptor id.
	providerPrefs store.Store[domain.AgentProviderPreference, string]
	// home resolves crowbar home for the descriptor catalog. It is the app-config
	// resolver, NOT a wsId lookup: providers are global, so provider resolution must
	// not depend on any workspace (the global PUT has no wsId to resolve one from).
	home func() (string, error)
	// installed is the install probe (defaults to Agent.Installed); injectable so
	// provider-resolution tests never depend on the host having claude/codex.
	installed func(a engineagents.Agent) bool
	// termWait detects the state no hook reports: a CLI parked on a modal Crowbar
	// cannot answer, which otherwise renders as an empty pane over a live process.
	// NIL when the terminal seam cannot render a screen, in which case every chat
	// reports the zero verdict — see newTerminalWaitDetector.
	termWait termwait.Detector

	// promptSettled fans out the edge where a delivery is retired without ever
	// having produced a turn. Wired at sweep start rather than at construction,
	// because the thing it publishes through is the hub — a layer above this one.
	// Nil until then, and nil forever in a daemon with no detector.
	promptSettled func(chatID, workspaceID, requestID string)

	// messageDelta fans a growing assistant message out to any client watching.
	// Wired at sweep start beside promptSettled and nil until then; a daemon with
	// nobody to publish to simply records the message when it finishes.
	messageDelta func(chatID, workspaceID, messageID, text string)
	// spawns serialises the USER-INITIATED spawn paths per chat (SpawnChat,
	// SwitchProvider, ResumeChat). See chatGate: it is the only thing that can stop two
	// concurrent switches putting two CLIs on one chat, and it is NEVER taken on the
	// hook path.
	spawns *chatGate
	// turns is the in-flight-turn registry a provider switch BLOCKS on, so it never quits
	// a CLI mid-answer. See turnWaits.
	turns *turnWaits
	// work is the authoritative process-local mirror of AgentChat.Working returned
	// by turn commands. Unlike GetChat's asynchronous projection, it cannot briefly
	// report idle after a hook has durably announced background work.
	work *chatWorkStates
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *chatGate
	// prompts is the durable at-most-once React-submission journal and the
	// process-local transition lock shared with user_prompt hook confirmation.
	prompts *promptJournal
	// pendingHooks is the fork-before-runner-persistence barrier. It buffers the
	// authenticated local hooks a provider can fire the instant its PTY starts,
	// then replays them in order once the runner row exists.
	pendingHooks *pendingRunnerHooks
	// messages assembles each assistant message from the increments its provider
	// streams, because the terminating hook carries only the LAST message of a
	// turn. See message_stream.go.
	messages *messageStreams
	// hookDeliveries durably deduplicates Crowbar relay retries before any turn
	// state or ledger mutation. The relay owns retry/spooling; this journal owns
	// the exactly-once ingress boundary.
	hookDeliveries *hookDeliveryJournal
	// catalogs owns only cancellation for in-flight deterministic probes. Results
	// are deliberately never cached.
	catalogs *catalogRuns
	// tools is the agent-facing capability surface DispatchMCP builds a per-call
	// ToolSet from. Its Chats port is always this usecase (set in New), so the one
	// dependency a caller can get wrong is the Resolver — and DispatchMCP refuses
	// to serve without it rather than quietly advertising an empty tool list.
	tools agenttools.Deps
	// answers is the desk of relays currently BLOCKED on a human. It is in memory
	// because a slot describes a live hook process holding a live provider gate
	// open; see answers.go.
	answers *answerDesk
	// minter issues the per-runner token an MCP call is authenticated by. It is
	// held here because the spawn path is what hands a runner its token, and a
	// runner's token must be minted by the same secret DispatchMCP verifies
	// against.
	minter *agenttools.TokenMinter
}

func New(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	activity agentactivity.EventStore,
	agents engineagents.Agents,
	term TerminalCommander,
	ws WorkspaceReader,
	lineage ChatLineage,
	providerPrefs store.Store[domain.AgentProviderPreference, string],
	home func() (string, error),
	installed func(a engineagents.Agent) bool,
	minter *agenttools.TokenMinter,
	tools agenttools.Deps,
) *Usecase {
	if installed == nil {
		installed = func(a engineagents.Agent) bool { return a.Installed() }
	}
	u := &Usecase{
		chats:          chats,
		runners:        runners,
		activity:       activity,
		telemetry:      newTelemetryStore(),
		messages:       newMessageStreams(),
		agents:         agents,
		term:           term,
		ws:             ws,
		lineage:        lineage,
		providerPrefs:  providerPrefs,
		home:           home,
		installed:      installed,
		spawns:         newChatGate(),
		turns:          newTurnWaits(),
		work:           newChatWorkStates(),
		turnStarts:     newChatGate(),
		prompts:        newPromptJournal(),
		pendingHooks:   newPendingRunnerHooks(),
		hookDeliveries: newHookDeliveryJournal(),
		catalogs:       newCatalogRuns(),
		answers:        newAnswerDesk(),
		tools:          tools,
		minter:         minter,
	}
	u.tools.Chats = u
	u.tools.ChatLogs = u
	// The per-provider tool switch, wired as a LIVE port rather than read once at
	// spawn: without it a chat spawned with tools on keeps them for the life of
	// its CLI, whatever the user does in Settings afterwards. See
	// agenttools.Deps.ToolAccess.
	u.tools.ToolAccess = u.providerMCPEnabled
	// Built LAST, and from u rather than from the arguments: two of its ports are
	// the usecase's own seams (the descriptor lookup, which needs u.home and
	// u.agents together). It only observes — nothing runs until
	// StartTerminalWaitSweep is called.
	u.termWait = newTerminalWaitDetector(u)
	return u
}
