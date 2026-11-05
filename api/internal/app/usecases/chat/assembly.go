package chat

import (
	"context"
	"time"

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
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Deps is everything the chat usecase is built over. The shared in-flight state (chat
// locks, answer desk) is deliberately absent: New builds it, because a second instance
// of either doubles a CLI in production.
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
	// Folders/Nodes let the placement-tree ancestor walks see past a folder, which
	// is not a Chat row. May be left nil.
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
	// ModelManifestFetchPrefs is the global toggle for whether a model.manifest:
	// source's background refresh may hit the network.
	ModelManifestFetchPrefs store.Store[domain.AgentModelManifestFetch, string]
	// ChatTelemetry backs the durable half of the telemetry cache: the last usage
	// report per chat, surviving a restart. Nil falls back to a process-local store.
	ChatTelemetry store.Store[domain.AgentChatTelemetry, string]
	// Home is the app-config crowbar-home resolver, NOT a wsId lookup: it resolves
	// the descriptor catalog, and providers are global.
	Home func() (string, error)
	// Ledger is every chat's ledger; a prompt is a row there before any CLI sees it.
	Ledger *ledgerstore.Store
	// Blobs holds the tool payloads the ledger's rows name.
	Blobs *ledgerstore.Blobs
	// DaemonAddress is the address the daemon bound, handed to every callback a
	// spawned CLI runs.
	DaemonAddress string
	// CrowbarBin is the crowbar binary a spawned CLI runs for its tool bridge;
	// empty is the one installed under the home.
	CrowbarBin string
	// NoAPITransport runs every provider over hooks alone.
	NoAPITransport bool
	// Installed is the install probe. Nil defaults to Agent.Installed, the real
	// one; only a test injects a stub, to isolate from the host PATH.
	Installed func(a engineagents.Agent) bool
	// Minter issues the per-runner token an MCP call is authenticated by. There is
	// exactly ONE per daemon: a runner's token must be minted by the same secret
	// that verifies it.
	Minter *agenttools.TokenMinter
	// Snapshots is the chat snapshot owner the composition root built and fed to the
	// fanout. Nil builds a private one, fed by nothing until a fanout is wired over it.
	Snapshots *snapshot.Snapshots
	// AgentWorking is told when a chat's busy answer flips, for its
	// workspace's working overlay.
	AgentWorking func(ctx context.Context, wsID, chatID string, working bool)
	// AnswerClock is the clock a held ask's answer budget runs out on.
	AnswerClock func(time.Duration) <-chan time.Time
	// Tools is the agent-facing capability surface. Its Chats, ChatLogs, Lineage,
	// ToolAccess and Resolver are filled in by New: they are this usecase's own
	// methods, and it does not exist when the caller builds the Deps.
	Tools agenttools.Deps
}

// shared is the in-flight state the components are built over, a value so "built
// exactly once" is visible: New makes one and hands every component the SAME one.
// None of it may be durable — each value describes a live process.
type shared struct {
	telemetry *telemetry.Store
	// locks is one lock per chat: every hook of the chat records under it, and
	// every user-initiated path decides under it. It is the only thing that can
	// stop two concurrent switches putting two CLIs on one chat.
	locks *ingress.Locks
	// writes is the ledger as every path that is not a hook writes it.
	writes ingress.Writer
	// answers is the desk of relays parked on a human decision.
	answers *answerdesk.Desk
}

// New builds the chat usecase and every component behind it.
func New(d Deps) *Usecase {
	// context.Background(): the telemetry hydration read runs once at boot, before
	// any request context exists.
	u := &Usecase{
		chats:     d.Chats,
		ledger:    d.Ledger,
		blobs:     d.Blobs,
		agents:    d.Agents,
		ws:        d.Workspace,
		worktree:  d.Worktree,
		tools:     d.Tools,
		folders:   d.Folders,
		nodes:     d.Nodes,
		repoRoots: d.RepoRoots,
		feeds:     newFeeds(feedCapacity),
	}
	locks := ingress.NewLocks()
	sh := shared{
		telemetry: telemetry.NewDurable(context.Background(), d.ChatTelemetry),
		locks:     locks,
		writes:    ingress.NewWriter(d.Ledger, locks),
	}
	u.asks = askResolver{writes: sh.writes}
	sh.answers = answerdesk.New(answerdesk.DefaultRetention, u.asks, answerdesk.WithBudgetClock(d.AnswerClock))
	u.answers = sh.answers
	// The tool surface's self-ports, filled in here because the usecase does
	// not exist when the caller builds the Deps.
	u.tools.Chats = u
	u.tools.ChatLogs = u
	u.tools.Lineage = u
	u.tools.ToolAccess = u.providerMCPEnabled
	u.tools.Resolver = agenttools.NewResolver(d.Minter, u, u.tools.Scope)

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

func (r snapshotReader) GetChats(ctx context.Context, chatIDs []string) ([]domain.Chat, error) {
	return r.chats.GetChats(ctx, chatIDs)
}

// buildComponents assembles the components and closes the edges between them. It is
// split out of New only for length; the two are one operation, never called twice.
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
		Chats:     d.Chats,
		Blobs:     d.Blobs,
		Telemetry: sh.telemetry,
		Agents:    d.Agents,
		Workspace: d.Workspace,
		Lineage:   d.Lineage,
		Home:      d.Home,
		Ledger:    d.Ledger,
		Writes:    sh.writes,

		DefaultPermissionLevel: u.DefaultPermissionLevel,
	})
	u.recorder = ingress.NewRecorder(ingress.RecorderDeps{
		Chats: d.Chats, Agents: d.Agents, Workspace: d.Workspace, Home: d.Home, Ledger: d.Ledger,
		Blobs: d.Blobs, Telemetry: sh.telemetry, Answers: sh.answers, Titles: u.conversations,
		Frames: hubFrames{feed: &u.feed, feeds: u.feeds},
	})
	u.runners = runner.New(runner.Deps{
		Chats:          d.Chats,
		Choices:        openAsks{ledger: d.Ledger},
		Agents:         d.Agents,
		Terminal:       d.Terminal,
		Workspace:      d.Workspace,
		AncestorCwd:    cwdResolver{chats: d.Chats, folders: d.Folders, nodes: d.Nodes, roots: d.RepoRoots},
		Ledger:         d.Ledger,
		DaemonHost:     d.DaemonAddress,
		CrowbarBin:     d.CrowbarBin,
		NoAPITransport: d.NoAPITransport,
		ChatLock:       sh.locks,
		Minter:         d.Minter,
		Answers:        sh.answers,
		Conversations:  u.conversations,
		Providers:      u.providers,
		Snapshots:      u.snapshots,
	})
	u.ingress = ingress.New(ingress.Deps{
		Runners: u.runners, Descriptors: u.recorder, Turns: u.recorder, Ledger: d.Ledger, Locks: sh.locks,
	})
	// The edges that can only close once both sides exist: purge retires the chat's
	// CLI, a hook resolves its runner through the live table, and the terminal-wait
	// detector reads screens through the hook ingress's classifiers.
	u.conversations.SetRunners(u.runners)
	u.recorder.SetRunners(u.runners)
	u.runners.SetTurns(runnerTurns{Recorder: u.recorder, ingress: u.ingress})
	// Subscribers run under the chat's ledger writer: they read snapshots only
	// through Held, which never reads the ledger.
	d.Ledger.Subscribe(func(chatID string, rows []ledger.Row, st ledger.State) {
		ctx := context.Background()
		flipped, wsID := u.snapshots.ApplyLedger(ctx, chatID, snapshot.FactsOf(st), movedTurn(rows, st))
		if flipped && d.AgentWorking != nil {
			d.AgentWorking(ctx, wsID, chatID, st.Busy())
		}
	})
	d.Ledger.Subscribe(func(chatID string, rows []ledger.Row, _ ledger.State) { u.feeds.rows(chatID, rows) })
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

// movedTurn is whether a row of the batch started or stopped the chat's newest
// turn. A late row moved nothing and announces nothing.
func movedTurn(rows []ledger.Row, st ledger.State) bool {
	for _, r := range rows {
		if !st.MovedTurn(r) {
			continue
		}
		if r.Kind == ledger.KindTurnStarted || r.Kind == ledger.KindTurnEnded || r.Kind == ledger.KindTurnFailed {
			return true
		}
	}
	return false
}
