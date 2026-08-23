package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TurnUsecase is the hook ingress and everything it writes: turns opening and
// closing, tool calls, subagents, interruptions, streamed assistant messages
// and provider telemetry.
//
// IT NEVER TAKES A SPAWN GATE. A hook must never block and must never fail — by
// the time it arrives the CLI has already acted — and the switch path parks on
// an in-flight turn with no timeout, releasable only from here. A path through
// this type that waited on the spawn gate would deadlock the CLI against its own
// hook and wedge every hook after it (see chatGate and awaitTurnComplete).
type TurnUsecase interface {
	// IngestHook applies one canonical hook event from a runner's CLI. It buffers
	// the event instead when the runner's row is still being persisted, so a
	// provider that fires the instant its PTY starts cannot overtake its own
	// session_start.
	IngestHook(
		ctx context.Context,
		runnerID string,
		provider string,
		canonicalEvent string,
		rawPayload []byte,
	) error

	// IngestHookDelivery is the exactly-once ingress for one RELAYED hook: it
	// dedupes the delivery, buffers it if the runner is still starting, runs its
	// effects, then durably records the completion.
	IngestHookDelivery(
		ctx context.Context,
		workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
		rawPayload []byte,
	) error

	// ReadActivity pages a chat's tool calls and returns its subagents,
	// interruptions and choices alongside them.
	ReadActivity(
		ctx context.Context,
		chatID string,
		after int64,
		limit int,
	) (ChatActivity, error)

	// ReadPendingChoices lists the questions a chat's CLI is still waiting on an
	// answer to.
	ReadPendingChoices(
		ctx context.Context,
		chatID string,
	) ([]domain.ActivityChoice, error)

	// ReadToolPayload returns one tool call's stored request or result body.
	// side is "result" for the result, anything else for the request.
	ReadToolPayload(
		ctx context.Context,
		chatID, toolID, side string,
	) ([]byte, error)

	// Telemetry reports the newest provider telemetry for a chat, if its CLI has
	// sent any. It is in memory because it describes a LIVE process.
	Telemetry(
		chatID string,
	) (engineagents.Telemetry, bool)

	// OpenWork reports whether the chat has a tool call or a subagent still
	// running. It is the second opinion the stall detector needs before it closes
	// a turn whose provider went quiet.
	OpenWork(
		ctx context.Context,
		chatID string,
	) (bool, error)

	// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen
	// is one of its modal prompts. An unresolvable home or descriptor is silent
	// rather than an error: it is a detector input, not a command.
	MatchTerminalPrompt(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalPrompt, bool)

	// MatchTerminalNotice asks a provider's descriptor whether a rendered screen
	// carries a notice worth recording against the stalled turn it closes.
	MatchTerminalNotice(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalNotice, bool)
}

var _ TurnUsecase = (*turnUsecase)(nil)

type turnUsecase struct {
	chats     agentchat.EventStore
	runners   agentrunner.EventStore
	activity  agentactivity.EventStore
	telemetry *telemetryStore
	agents    engineagents.Agents
	ws        WorkspaceReader
	home      func() (string, error)
	// work is the authoritative process-local mirror of AgentChat.Working returned
	// by the turn commands. Unlike GetChat's asynchronous projection, it cannot
	// briefly report idle after a hook has durably announced background work.
	work *chatWorkStates
	// turns is the in-flight-turn registry a provider switch BLOCKS on, so it
	// never quits a CLI mid-answer. See turnWaits.
	turns *turnWaits
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *chatGate
	// messages assembles each assistant message from the increments its provider
	// streams, because the terminating hook carries only the LAST message of a
	// turn. See message_stream.go.
	messages *messageStreams
	// pendingHooks is the fork-before-runner-persistence barrier. It buffers the
	// authenticated local hooks a provider can fire the instant its PTY starts,
	// then replays them in order once the runner row exists.
	pendingHooks *pendingRunnerHooks
	// hookDeliveries durably deduplicates Crowbar relay retries before any turn
	// state or ledger mutation. The relay owns retry/spooling; this journal owns
	// the exactly-once ingress boundary.
	hookDeliveries agentjournal.HookDeliveries
	// hookGates serialises one runner's hook ingestion. It is held across the
	// WHOLE ingest — dedupe, replay buffering, effects, completion — which is why
	// it lives here rather than inside the delivery journal.
	hookGates *chatGate
	chat      *chatUsecase
	// runner is reached for the placement half of a hook (a session_start move,
	// an eviction) and for the prompt journal a user_prompt confirms. NOTHING
	// here may call a runner path that takes the spawn gate: SwitchProvider holds
	// it while parked on a turn only this type can release, so a hook that waited
	// on it would deadlock against the very switch waiting on the hook.
	runner  *runnerUsecase
	answers *answerUsecase
	// messageDelta fans a growing assistant message out to any client watching.
	// Wired at sweep start rather than at construction, because the thing it
	// publishes through is the hub — a layer above this one. Nil until then, and
	// nil forever in a daemon with no detector.
	messageDelta func(chatID, workspaceID, messageID, text string)
}

func newTurnUsecase(
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	activity agentactivity.EventStore,
	telemetry *telemetryStore,
	agents engineagents.Agents,
	ws WorkspaceReader,
	home func() (string, error),
	work *chatWorkStates,
	turns *turnWaits,
	turnStarts *chatGate,
	messages *messageStreams,
	pendingHooks *pendingRunnerHooks,
	hookDeliveries agentjournal.HookDeliveries,
	hookGates *chatGate,
	answers *answerUsecase,
) *turnUsecase {
	return &turnUsecase{
		chats:          chats,
		runners:        runners,
		activity:       activity,
		telemetry:      telemetry,
		agents:         agents,
		ws:             ws,
		home:           home,
		work:           work,
		turns:          turns,
		turnStarts:     turnStarts,
		messages:       messages,
		pendingHooks:   pendingHooks,
		hookDeliveries: hookDeliveries,
		hookGates:      hookGates,
		answers:        answers,
	}
}
