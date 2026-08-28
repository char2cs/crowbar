package chat

import (
	"context"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatUsecase owns the chat aggregate — its identity, title, model selection
// and hard deletion — and every read served off the conversation record it
// accumulates.
//
// It starts no processes. A chat exists, and is readable, whether or not a CLI
// has ever run on it: minting one, renaming it and reading its log are all
// answerable with no runner in sight, which is why they are separable from the
// runner lifecycle at all.
type ChatUsecase interface {
	// MintChat creates an empty chat in a workspace and returns its id. No CLI is
	// started: the chat is dormant until a runner is placed on it.
	MintChat(
		ctx context.Context,
		workspaceID string,
	) (string, error)

	// RenameChat retitles a chat, honouring where the title came from: a
	// "derived" title never overwrites an existing one, an "agent" title never
	// overwrites a locked one, and anything else is a manual rename that wins and
	// locks.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	// RenameByRunner retitles whichever chat a runner is currently placed on. A
	// runner placed nowhere renames nothing.
	RenameByRunner(
		ctx context.Context,
		runnerID, title, source string,
	) error

	// PurgeChat hard-deletes a chat: the aggregate, its conversation record, its
	// telemetry, its conversation history and its on-disk footprint, retiring
	// every CLI still on it.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error

	// ListChats returns every chat the daemon knows, across all workspaces.
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)

	// ListChatsByWorkspace returns one workspace's chats.
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	// GetChat reads one chat aggregate.
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

	// SetChatSelection records the model and effort the chat's next CLI is to be
	// launched with, refusing a value the resolved provider does not declare with
	// apperr.ErrInvalidArgument.
	SetChatSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) error

	// ReadChatLog renders a chat's whole conversation as speaker/body turns. It is
	// the read behind the get_chat_log tool one agent uses to read another chat.
	ReadChatLog(
		ctx context.Context,
		chatID string,
	) ([]agenttools.ChatTurn, error)

	// ReadMessages pages the conversation record, newest page last. after and
	// before are mutually exclusive cursors.
	ReadMessages(
		ctx context.Context,
		chatID string,
		after, before, limit int,
	) (domain.LedgerPage, error)

	// NoteThreadLineage records, in the chat's own conversation, that it has been
	// moved under new parents from this point on. It appends nothing to a chat
	// that has said nothing yet.
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error

	// Ancestors returns the CHAT ancestors of chatID, nearest parent first —
	// what a thread inherits. Folders are transparent to it, so a thread filed
	// two folders deep under a chat inherits exactly what it would sitting
	// directly under it. Empty for a chat at the panel root.
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)

	// AssembleHandoff renders the conversation — capped to the most recent
	// turns, see RecentHandoffWindow — into the document an incoming CLI is
	// spawned with. It returns the empty string when there is nothing to hand
	// over.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
}

var _ ChatUsecase = (*Usecase)(nil)

// Usecase is the chat feature: one type, whose methods are exposed across five
// files by responsibility — the chat record here, the turn in turn.go, the vendor
// CLI in runner.go, the human-in-the-loop in answers.go and the provider table in
// providers.go.
//
// Each of those five files opens with the port it satisfies and the sentinels its
// methods return, so a consumer reads one file to learn one responsibility. They
// are five ports and not one because a consumer should not be able to reach past
// what it needs: a route that lists chats has no business starting a CLI. The
// surface is split; the state is not.
//
// It is ONE type and not five because those five responsibilities are one
// surface that calls itself in a cycle: a purge retires the CLIs on the chat, a
// hook moves a runner, a switch waits on a turn only the hook path can end. Five
// types could only express that by binding each other after construction, which
// is what this replaced.
//
// The work itself is NOT here. It lives in four components under internal/, and
// those really are separate: each declares the narrow port it needs from a peer
// and never imports one (see layering_test.go). This type is where they are built,
// wired, and named for the outside — nothing more.
type Usecase struct {
	// The stores and seams the answer path reads directly. Everything else this
	// type needs, it needs only long enough to build a component out of it.
	chats       agentchat.EventStore
	runnerStore agentrunner.EventStore
	activity    agentactivity.EventStore
	agents      engineagents.Agents
	ws          WorkspaceReader
	// answers is the desk of relays currently BLOCKED on a human. It is in memory
	// because a slot describes a live hook process holding a live provider gate
	// open; see answers.go.
	answers *answerdesk.Desk
	// permissionLevels is the per-chat trust dial SetChatPermissionLevel writes
	// directly — the same pointer Task 4 wired into turn.Deps.
	permissionLevels *permission.Store
	// tools is the agent-facing capability surface. It is kept after construction
	// for one reason: its Chats, ChatLogs, Lineage and ToolAccess ports are this
	// type's own methods, so New is the only thing that can fill them, and a nil
	// ToolAccess FAILS OPEN — the per-provider Tools switch the user turned off
	// would be silently back on. Keeping the value is what lets that wiring be
	// guarded rather than assumed.
	tools agenttools.Deps

	// The five components. Each owns one responsibility, and the delegating
	// methods in this file and the other five are the whole of what reaches them.
	conversations *conversation.Conversations
	turns         *turn.Turns
	runners       *runner.Runners
	providers     *provider.Providers
	defaultLevel  *defaultlevel.DefaultLevel
}

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
	// permissionLevels is the per-chat trust dial an auto-resolve decision reads.
	permissionLevels *permission.Store
}

// New builds the chat usecase and every component behind it.
func New(d Deps) *Usecase {
	sh := shared{
		telemetry:        telemetry.New(),
		work:             inflight.NewWork(),
		spawns:           inflight.NewGate(),
		turns:            inflight.NewTurns(),
		turnStarts:       inflight.NewGate(),
		pendingHooks:     inflight.NewHooks(),
		answers:          answerdesk.New(answerdesk.DefaultRetention, d.Activity),
		permissionLevels: permission.New(),
	}
	u := &Usecase{
		chats:            d.Chats,
		runnerStore:      d.Runners,
		activity:         d.Activity,
		agents:           d.Agents,
		ws:               d.Workspace,
		answers:          sh.answers,
		permissionLevels: sh.permissionLevels,
		tools:            d.Tools,
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

		PermissionLevels:       sh.permissionLevels,
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

		PermissionLevels:       sh.permissionLevels,
		DefaultPermissionLevel: u.DefaultPermissionLevel,

		Conversations: u.conversations,
	})
	u.runners = runner.New(runner.Deps{
		Chats:         d.Chats,
		Runners:       d.Runners,
		Activity:      d.Activity,
		Agents:        d.Agents,
		Terminal:      d.Terminal,
		Workspace:     d.Workspace,
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

// The chat record. A chat exists, and is readable, whether or not a CLI has ever
// run on it — everything here is answerable with no runner in sight.

// MintChat creates an empty chat in a workspace and returns its id.
func (u *Usecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	return u.conversations.MintChat(ctx, workspaceID)
}

// RenameChat retitles a chat under the user > agent > derived precedence.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	return u.conversations.RenameChat(ctx, chatID, title, source)
}

// RenameByRunner retitles whichever chat a runner is currently placed on.
func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	return u.conversations.RenameByRunner(ctx, runnerID, title, source)
}

// PurgeChat hard-deletes a chat and retires every CLI still on it.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	return u.conversations.PurgeChat(ctx, chatID)
}

// ListChats returns every chat in the daemon.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return u.conversations.ListChats(ctx)
}

// ListChatsByWorkspace returns the chats anchored to one workspace.
func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return u.conversations.ListChatsByWorkspace(ctx, workspaceID)
}

// GetChat returns one chat by id.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return u.conversations.GetChat(ctx, id)
}

// SetChatSelection pins the model and reasoning effort the chat's next CLI is
// launched with.
func (u *Usecase) SetChatSelection(
	ctx context.Context,
	chatID, model, effort string,
) error {
	return u.conversations.SetChatSelection(ctx, chatID, model, effort)
}

// ReadChatLog returns the chat's turns as the tool surface renders them.
func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	return u.conversations.ReadChatLog(ctx, chatID)
}

// ReadMessages returns one page of the chat's turns.
func (u *Usecase) ReadMessages(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (domain.LedgerPage, error) {
	return u.conversations.ReadMessages(ctx, chatID, after, before, limit)
}

// NoteThreadLineage records, in the chat's own log, which chats it was threaded
// under.
func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	return u.conversations.NoteThreadLineage(ctx, chatID, ancestors)
}

// AssembleHandoff renders the chat's ledger into the prior context a freshly
// spawned CLI can be given.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.conversations.AssembleHandoff(ctx, chatID)
}

// Ancestors returns the CHAT ancestors of chatID, nearest parent first — what a
// thread inherits.
func (u *Usecase) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	return u.conversations.Ancestors(ctx, chatID)
}

// RemoveUnderHome deletes target only when it is strictly under crowbar home,
// and never fails the caller.
//
// It is exported because the workspace-delete cascade reaps a chat's on-disk
// footprint from the app layer, off the same path resolution and the same guard
// PurgeChat uses — reimplementing either there is how a delete ends up pointed at
// the user's real repository.
func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	worktreepath.RemoveUnderHome(ctx, home, target)
}
