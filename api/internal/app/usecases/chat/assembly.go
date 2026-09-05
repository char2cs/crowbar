package chat

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Deps is everything the chat usecase is built over: the three stores it reads
// and writes, the engine it resolves providers through, the seams onto the rest
// of the daemon, and the agent-facing tool surface.
//
// What is NOT here is any of the shared in-flight state — the gates, the turn
// registry, the work mirror, the journals. New builds those, and a caller must
// not be able to supply them: a second instance of any one does not fail to
// compile and does not fail a test that exercises a single path, it wedges a
// switch or doubles a CLI in production.
type Deps struct {
	Chats    agentchat.EventStore
	Runners  agentrunner.EventStore
	Activity agentactivity.EventStore
	Agents   engineagents.Agents
	Terminal TerminalCommander
	// Workspace resolves the on-disk locations one workspace's agent work happens
	// in. It is the only seam this feature has onto the workspace layer.
	Workspace WorkspaceReader
	Worktree  WorktreeCreator // Promote's seam onto the worktree hierarchy usecase
	// Lineage answers "what does this chat read" at spawn time.
	Lineage ChatLineage
	// ProviderPrefs is the global (per user/machine) provider priority+enabled table.
	ProviderPrefs store.Store[domain.AgentProviderPreference, string]
	// PermissionPrefs is the global default permission level a new chat is
	// seeded with.
	PermissionPrefs store.Store[domain.AgentPermissionDefault, string]
	// Home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog, and providers are global.
	Home func() (string, error)
	// Installed is the install probe. Nil defaults to Agent.Installed, the real
	// one; only a test injects a stub, to isolate from the host PATH.
	Installed func(a engineagents.Agent) bool
	// Minter issues the per-runner token an MCP call is authenticated by. There is
	// exactly ONE per daemon: a runner's token must be minted by the same secret
	// that verifies it.
	Minter *agenttools.TokenMinter
	// Tools is the agent-facing capability surface. Its Chats, ChatLogs, Lineage
	// and ToolAccess ports are filled in by New, because they are this usecase's
	// own methods and it does not exist when the caller builds the Deps.
	Tools agenttools.Deps
}

// shared is the in-flight state the components are built over.
//
// It exists as a value so that "built exactly once" is something you can see
// rather than a rule you have to trust: New makes one, and every component that
// needs a piece of it is handed the SAME one. None of it is durable and none of
// it may be — each value describes a live process, and a daemon restart kills
// every process it could have been describing.
type shared struct {
	telemetry *telemetry.Store
	// work mirrors the authoritative Working flag the turn commands return. Unlike
	// the projection, it cannot briefly report idle after a hook has durably
	// announced background work.
	work *inflight.Work
	// spawns serialises the USER-INITIATED spawn paths per chat. It is the only
	// thing that can stop two concurrent switches putting two CLIs on one chat, and
	// it is NEVER taken on the hook path.
	spawns *inflight.Gate
	// turns is the in-flight-turn registry a provider switch BLOCKS on, so it never
	// quits a CLI mid-answer.
	turns *inflight.Turns
	// turnStarts makes a hook's durable turn start atomic with the final
	// idle-check-and-displace section of destructive TUI replacement.
	turnStarts *inflight.Gate
	// pendingHooks is the fork-before-runner-persistence barrier: the spawn path
	// installs it before the fork and finishes it once the runner row exists, and
	// the ingest path buffers into it meanwhile.
	pendingHooks *inflight.Hooks
	// answers is the desk of relays parked on a human decision.
	answers *answerdesk.Desk
}

// New builds the chat usecase and every component behind it.
func New(d Deps) *Usecase {
	sh := shared{
		telemetry:    telemetry.New(),
		work:         inflight.NewWork(),
		spawns:       inflight.NewGate(),
		turns:        inflight.NewTurns(),
		turnStarts:   inflight.NewGate(),
		pendingHooks: inflight.NewHooks(),
		answers:      answerdesk.New(answerdesk.DefaultRetention, d.Activity),
	}
	u := &Usecase{
		chats:       d.Chats,
		runnerStore: d.Runners,
		activity:    d.Activity,
		agents:      d.Agents,
		ws:          d.Workspace,
		worktree:    d.Worktree,
		answers:     sh.answers,
		tools:       d.Tools,
		work:        sh.work,
	}
	// The tool surface's four self-ports, filled in here because the usecase does
	// not exist when the caller builds the Deps.
	u.tools.Chats = u
	u.tools.ChatLogs = u
	u.tools.Lineage = u
	u.tools.ToolAccess = u.providerMCPEnabled

	u.buildComponents(d, sh)
	return u
}

// buildComponents assembles the four halves of the surface and closes the edges
// between them.
//
// It is split out of New only to keep each within its length budget; the two are
// one operation, and nothing may call this twice.
func (u *Usecase) buildComponents(d Deps, sh shared) {
	u.providers = provider.New(provider.Deps{
		Agents:    d.Agents,
		Home:      d.Home,
		Installed: d.Installed,
		Prefs:     d.ProviderPrefs,
		Minter:    d.Minter,
		Tools:     u.tools,
	})
	u.defaultLevel = defaultlevel.New(defaultlevel.Deps{Prefs: d.PermissionPrefs})
	u.conversations = conversation.New(conversation.Deps{
		Chats:     d.Chats,
		Runners:   d.Runners,
		Activity:  d.Activity,
		Telemetry: sh.telemetry,
		Agents:    d.Agents,
		Workspace: d.Workspace,
		Lineage:   d.Lineage,
		Home:      d.Home,
		Work:      sh.work,
		Spawns:    sh.spawns,

		DefaultPermissionLevel: u.DefaultPermissionLevel,
	})
	u.turns = turn.New(turn.Deps{
		Chats:         d.Chats,
		Runners:       d.Runners,
		Activity:      d.Activity,
		Telemetry:     sh.telemetry,
		Agents:        d.Agents,
		Workspace:     d.Workspace,
		Home:          d.Home,
		Work:          sh.work,
		InflightTurns: sh.turns,
		TurnStarts:    sh.turnStarts,
		PendingHooks:  sh.pendingHooks,
		Answers:       sh.answers,

		Conversations: u.conversations,
	})
	u.runners = runner.New(runner.Deps{
		Chats:         d.Chats,
		Runners:       d.Runners,
		Activity:      d.Activity,
		Agents:        d.Agents,
		Terminal:      d.Terminal,
		Workspace:     d.Workspace,
		AncestorCwd:   cwdResolver{chats: d.Chats},
		Home:          d.Home,
		Spawns:        sh.spawns,
		InflightTurns: sh.turns,
		TurnStarts:    sh.turnStarts,
		Work:          sh.work,
		PendingHooks:  sh.pendingHooks,
		Minter:        d.Minter,
		Answers:       sh.answers,
		Conversations: u.conversations,
		Providers:     u.providers,
	})
	// The three edges that can only close once both sides exist: a purge retires
	// the CLIs on the chat (and a failed spawn discards the chat it minted), a hook
	// applies a placement the CLI already performed, and the terminal-wait detector
	// reads screens through the hook ingress's own classifiers.
	u.conversations.SetRunners(u.runners)
	u.turns.SetRunners(u.runners)
	u.runners.SetTurns(u.turns)
}
