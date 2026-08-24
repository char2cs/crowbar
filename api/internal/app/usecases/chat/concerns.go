package chat

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Concerns is the agent surface as five separate ports: the chat aggregate, the
// activity record hooks write, the runner lifecycle, the human-in-the-loop
// answer desk and the provider table.
//
// It is a construction bundle, not a facade — nothing here forwards a call.
// They travel together only because agent.New is the one place that can build
// them: they share the spawn gate, the turn registry, the work mirror and the
// journals, and a second instance of any of those is a silent wedge. A consumer
// takes the one or two ports it actually needs.
type Concerns struct {
	Chat     ChatUsecase
	Turn     TurnUsecase
	Runner   RunnerUsecase
	Answer   AnswerUsecase
	Provider ProviderUsecase
}

// New builds every agent concern and returns the five as one value.
//
// The shared machinery — the spawn gate, the turn registry, the work mirror,
// the turn-start interlock, the hook barrier, the message streams and the two
// journals — is constructed HERE, once, and the same value is handed to each
// concern that needs it. A second instance of any of them is a silent wedge: two
// turn registries means a switch parks on a turn nothing will ever complete, and
// two spawn gates means two CLIs on one chat.
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
) Concerns {
	telemetry := newTelemetryStore()
	work := newChatWorkStates()
	spawns := newChatGate()
	turns := newTurnWaits()
	turnStarts := newChatGate()
	pendingHooks := newPendingRunnerHooks()
	answers := newAnswerUsecase(activity, chats, runners, agents, ws)
	chat := newChatUsecase(
		chats, runners, activity, telemetry, agents, ws, lineage, home, work, spawns,
	)
	turn := newTurnUsecase(
		chats, runners, activity, telemetry, agents, ws, home, work, turns, turnStarts,
		newMessageStreams(), pendingHooks, agentjournal.NewHookDeliveries(), newChatGate(),
		answers,
	)
	runner := newRunnerUsecase(
		chats, runners, activity, agents, term, ws, spawns, turns, turnStarts, work,
		agentjournal.NewPromptRequests(), pendingHooks, newCatalogRuns(), minter, answers,
	)
	// The tool surface's two self-ports, filled in here because the chat concern
	// does not exist when the caller builds the Deps. newProviderUsecase adds the
	// third, ToolAccess, which it can only bind to a method of its own.
	tools.Chats = chat
	tools.ChatLogs = chat
	providers := newProviderUsecase(agents, home, installed, providerPrefs, minter, tools)

	// The concerns reach each other directly, and the graph has cycles in it (a
	// spawn discards its half-created chat; a purge retires the CLIs on it), so
	// the sibling ports are bound after all of them exist rather than passed into
	// constructors that could not name each other.
	chat.runner = runner
	turn.chat = chat
	turn.runner = runner
	runner.chat = chat
	runner.turn = turn
	runner.providers = providers

	// Built LAST, from the concerns rather than from the arguments: its ports are
	// spread across them and it binds them by value. It only observes — nothing
	// runs until StartTerminalWaitSweep is called.
	runner.termWait = newTerminalWaitDetector(chat, turn, runner)
	return Concerns{
		Chat:     chat,
		Turn:     turn,
		Runner:   runner,
		Answer:   answers,
		Provider: providers,
	}
}
