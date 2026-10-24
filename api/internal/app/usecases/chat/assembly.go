package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/manifestfetch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Deps is everything the chat usecase is built over: the three stores it reads
// and writes, the engine it resolves providers through, the seams onto the rest
// of the daemon, and the agent-facing tool surface.
//
// What is NOT here is any of the shared in-flight state — the chat locks, the
// answer desk. New builds those, and a caller must not be able to supply them:
// a second instance of either doubles a CLI in production.
type Deps struct {
	Chats    agentchat.EventStore
	Agents   engineagents.Agents
	Terminal TerminalCommander
	// Workspace resolves the on-disk locations one workspace's agent work happens
	// in. It is the only seam this feature has onto the workspace layer.
	Workspace WorkspaceReader
	Worktree  WorktreeCreator // Promote's seam onto the worktree hierarchy usecase
	// Lineage answers "what does this chat read" at spawn time.
	Lineage ChatLineage
	// Folders/Nodes let own_worktree.go/promote.go/repo_scope.go/
	// cwd_resolver.go's ancestor walks see past a Folder-only ancestor
	// (2026-09-08 sidebar-placement-unification Task 8's own review fix
	// round) — see Usecase's own doc for why. May be left nil.
	Folders TreeFolders
	Nodes   TreeNodes
	// RepoRoots finishes those same walks for a row filed in a repo-root
	// folder, which has no workspace-owning row above it. May be left nil.
	RepoRoots TreeRepoRoots
	// ProviderPrefs is the global (per user/machine) provider priority+enabled table.
	ProviderPrefs store.Store[domain.AgentProviderPreference, string]
	// PermissionPrefs is the global default permission level a new chat is
	// seeded with.
	PermissionPrefs store.Store[domain.AgentPermissionDefault, string]
	// ModelManifestFetchPrefs is the global toggle for whether a
	// model.manifest: source's background refresh may hit the network —
	// wired straight into Agents.SetManifestFetchEnabled below, since that
	// engine-side getter is pulled fresh on every refresh rather than pushed
	// on change.
	ModelManifestFetchPrefs store.Store[domain.AgentModelManifestFetch, string]
	// ChatTelemetry backs the durable half of the telemetry cache — the last
	// usage report per chat, surviving a daemon restart. Nil is a valid,
	// supported value (every test that builds Deps by hand): New falls back to
	// a process-local-only store, same as before this field existed.
	ChatTelemetry store.Store[domain.AgentChatTelemetry, string]
	// Home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog, and providers are global.
	Home func() (string, error)
	// Ledger is every chat's ledger; a prompt is a row there before any CLI sees it.
	Ledger *ledgerstore.Store
	// Blobs holds the tool payloads the ledger's rows name.
	Blobs *ledgerstore.Blobs
	// DaemonSocket is the unix socket the daemon bound, handed to every callback a
	// spawned CLI runs; empty over TCP.
	DaemonSocket string
	// Installed is the install probe. Nil defaults to Agent.Installed, the real
	// one; only a test injects a stub, to isolate from the host PATH.
	Installed func(a engineagents.Agent) bool
	// Minter issues the per-runner token an MCP call is authenticated by. There is
	// exactly ONE per daemon: a runner's token must be minted by the same secret
	// that verifies it.
	Minter *agenttools.TokenMinter
	// Snapshots is the chat snapshot owner the composition root built and fed
	// to the fanout. Nil builds a private one (tests), fed by nothing until a
	// test wires the fanout over Snapshots().
	Snapshots *snapshot.Snapshots
	// AgentWorking is told when a chat's busy answer flips, for its
	// workspace's working overlay.
	AgentWorking func(ctx context.Context, wsID, chatID string, working bool)
	// Tools is the agent-facing capability surface. Its Chats, ChatLogs, Lineage,
	// ToolAccess and Resolver are filled in by New: they are this usecase's own
	// methods, and it does not exist when the caller builds the Deps.
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
	// locks is one lock per chat: every hook of the chat records under it, and
	// every user-initiated path decides under it. It is the only thing that can
	// stop two concurrent switches putting two CLIs on one chat.
	locks *ingress.Locks
	// answers is the desk of relays parked on a human decision.
	answers *answerdesk.Desk
}

// New builds the chat usecase and every component behind it.
func New(d Deps) *Usecase {
	// context.Background(): this hydration read happens exactly once, at
	// daemon boot, before there is a request context to carry — same
	// justification as the other startup-only Background() reads in this
	// codebase (e.g. container.go's sweepTargets).
	u := &Usecase{
		chats:      d.Chats,
		ledger:     d.Ledger,
		blobs:      d.Blobs,
		transcript: transcript.New(d.Ledger),
		agents:     d.Agents,
		ws:         d.Workspace,
		worktree:   d.Worktree,
		tools:      d.Tools,
		folders:    d.Folders,
		nodes:      d.Nodes,
		repoRoots:  d.RepoRoots,
	}
	sh := shared{
		telemetry: telemetry.NewDurable(context.Background(), d.ChatTelemetry),
		locks:     ingress.NewLocks(),
		answers: answerdesk.New(
			answerdesk.DefaultRetention, askResolver{ledger: d.Ledger}, answerdesk.WithChange(u.announceChoices),
		),
	}
	u.answers = sh.answers
	// The tool surface's self-ports, filled in here because the usecase does
	// not exist when the caller builds the Deps.
	u.tools.Chats = u
	u.tools.ChatLogs = u
	u.tools.Lineage = u
	u.tools.ToolAccess = u.providerMCPEnabled
	u.tools.Resolver = agenttools.NewResolver(d.Minter, u, u.tools.ChatReads, u.tools.Scope)

	u.snapshots = d.Snapshots
	if u.snapshots == nil {
		u.snapshots = snapshot.NewAtBoot()
	}
	u.buildComponents(d, sh)
	if d.Nodes != nil {
		u.snapshots.SetCorrect(func(ctx context.Context, chats []domain.Chat) []domain.Chat {
			return correctHomeChats(ctx, d.Nodes, chats)
		})
	}
	u.snapshots.Bind(snapshotReader{chats: d.Chats, ledger: d.Ledger}, u.runners)
	return u
}

// snapshotReader is the read model the snapshot owner falls back to for a
// chat no event has touched since boot.
type snapshotReader struct {
	chats  agentchat.EventStore
	ledger *ledgerstore.Store
}

func (r snapshotReader) ViewLedger(ctx context.Context, chatID string, fn func(ledger.State) error) error {
	return r.ledger.View(ctx, chatID, fn)
}

func (r snapshotReader) GetChat(ctx context.Context, chatID string) (domain.Chat, error) {
	return r.chats.GetChat(ctx, chatID)
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
	u.manifestFetch = manifestfetch.New(manifestfetch.Deps{Prefs: d.ModelManifestFetchPrefs})
	// Pull, not push: the engine reads this getter fresh on every
	// model.manifest: refresh attempt, so a settings change takes effect on
	// the very next one rather than needing this wiring to run again.
	if d.Agents != nil {
		d.Agents.SetManifestFetchEnabled(func() bool {
			enabled, err := u.manifestFetch.Get(context.Background())
			if err != nil {
				return true
			}
			return enabled
		})
	}
	u.conversations = conversation.New(conversation.Deps{
		Chats:      d.Chats,
		Transcript: u.transcript,
		Blobs:      d.Blobs,
		Telemetry:  sh.telemetry,
		Agents:     d.Agents,
		Workspace:  d.Workspace,
		Lineage:    d.Lineage,
		Home:       d.Home,
		Ledger:     d.Ledger,

		DefaultPermissionLevel: u.DefaultPermissionLevel,
	})
	u.recorder = ingress.NewRecorder(ingress.RecorderDeps{
		Chats: d.Chats, Agents: d.Agents, Workspace: d.Workspace, Home: d.Home, Ledger: d.Ledger,
		Blobs: d.Blobs, Telemetry: sh.telemetry, Answers: sh.answers, Titles: u.conversations,
	})
	u.runners = runner.New(runner.Deps{
		Chats:         d.Chats,
		Choices:       u.transcript,
		Agents:        d.Agents,
		Terminal:      d.Terminal,
		Workspace:     d.Workspace,
		AncestorCwd:   cwdResolver{chats: d.Chats, folders: d.Folders, nodes: d.Nodes, roots: d.RepoRoots},
		Ledger:        d.Ledger,
		DaemonSocket:  d.DaemonSocket,
		ChatLock:      sh.locks,
		Minter:        d.Minter,
		Answers:       sh.answers,
		Conversations: u.conversations,
		Providers:     u.providers,
		Snapshots:     u.snapshots,
	})
	u.ingress = ingress.New(ingress.Deps{
		Runners: u.runners, Descriptors: u.recorder, Turns: u.recorder, Ledger: d.Ledger, Locks: sh.locks,
	})
	// The edges that can only close once both sides exist: a purge retires the
	// CLI on the chat (and a failed spawn discards the chat it minted), a hook
	// resolves its runner through the live table, and the terminal-wait detector
	// reads screens through the hook ingress's own classifiers.
	u.conversations.SetRunners(u.runners)
	u.recorder.SetRunners(u.runners)
	u.runners.SetTurns(runnerTurns{Recorder: u.recorder, ingress: u.ingress})
	// Subscribers run under the chat's ledger writer: they read snapshots only
	// through Held, which never reads the ledger.
	d.Ledger.Subscribe(func(chatID string, rows []ledger.Row, st ledger.State) {
		ctx := context.Background()
		flipped, wsID := u.snapshots.ApplyLedger(ctx, chatID, snapshot.FactsOf(st), turnKind(rows, st))
		if flipped && d.AgentWorking != nil {
			d.AgentWorking(ctx, wsID, chatID, st.Busy())
		}
	})
	d.Ledger.Subscribe(u.publishLedgerRows)
}

// runnerTurns is the hook side as the runner lifecycle sees it: the recorder,
// and the ingress its api connections place their events through.
type runnerTurns struct {
	*ingress.Recorder
	ingress *ingress.Ingress
}

// Hook places one event an api connection resolved, and returns the ask its
// answer is held on.
func (t runnerTurns) Hook(ctx context.Context, runnerID, provider, event string, raw []byte) (string, error) {
	reply, err := t.ingress.Hook(ctx, runnerID, provider, event, raw, engineagents.ChannelAPI)
	return reply.Ask, err
}

// turnKind is the chat frame kind the batch's last row that moved the chat's
// newest turn announces: a turn started, or one stopped; "" for a batch with
// none. A late row moved nothing and announces nothing.
func turnKind(rows []ledger.Row, st ledger.State) string {
	kind := ""
	for _, r := range rows {
		if !st.MovedTurn(r) {
			continue
		}
		if r.Kind == ledger.KindTurnStarted {
			kind = "turn_started"
		}
		if r.Kind == ledger.KindTurnEnded || r.Kind == ledger.KindTurnFailed {
			kind = "turn_stopped"
		}
	}
	return kind
}
