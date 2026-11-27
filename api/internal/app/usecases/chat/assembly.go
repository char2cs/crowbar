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

type Deps struct {
	Chats                   agentchat.EventStore
	Agents                  engineagents.Agents
	Terminal                TerminalCommander
	Workspace               WorkspaceReader
	Worktree                WorktreeCreator
	Lineage                 ChatLineage
	Folders                 TreeFolders
	Nodes                   TreeNodes
	RepoRoots               TreeRepoRoots
	ProviderPrefs           store.Store[domain.AgentProviderPreference, string]
	PermissionPrefs         store.Store[domain.AgentPermissionDefault, string]
	ModelManifestFetchPrefs store.Store[domain.AgentModelManifestFetch, string]
	ChatTelemetry           store.Store[domain.AgentChatTelemetry, string]
	Home                    func() (string, error)
	Ledger                  *ledgerstore.Store
	Blobs                   *ledgerstore.Blobs
	DaemonAddress           string
	CrowbarBin              string
	NoAPITransport          bool
	Installed               func(a engineagents.Agent) bool
	Minter                  *agenttools.TokenMinter
	Snapshots               *snapshot.Snapshots
	AgentWorking            func(ctx context.Context, wsID, chatID string, working bool)
	AnswerClock             func(time.Duration) <-chan time.Time
	Tools                   agenttools.Deps
}

type shared struct {
	telemetry *telemetry.Store
	locks     *ingress.Locks
	writes    ingress.Writer
	answers   *answerdesk.Desk
}

func New(d Deps) *Usecase {
	u := &Usecase{
		chats:     d.Chats,
		ledger:    d.Ledger,
		blobs:     d.Blobs,
		agents:    d.Agents,
		ws:        d.Workspace,
		home:      d.Home,
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
	u.tools.Chats = u
	u.tools.ChatLogs = u
	u.tools.Branches = u
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
		Home:           d.Home,
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
	u.conversations.SetRunners(u.runners)
	u.recorder.SetRunners(u.runners)
	u.runners.SetTurns(runnerTurns{Recorder: u.recorder, ingress: u.ingress})
	d.Ledger.Subscribe(func(chatID string, rows []ledger.Row, st ledger.State) {
		ctx := context.Background()
		flipped, wsID := u.snapshots.ApplyLedger(ctx, chatID, snapshot.FactsOf(st), movedTurn(rows, st))
		if flipped && d.AgentWorking != nil {
			d.AgentWorking(ctx, wsID, chatID, st.Busy())
		}
	})
	d.Ledger.Subscribe(func(chatID string, rows []ledger.Row, _ ledger.State) { u.feeds.rows(chatID, rows) })
}

type runnerTurns struct {
	*ingress.Recorder
	ingress *ingress.Ingress
}

func (t runnerTurns) Hook(ctx context.Context, runnerID, provider, event string, raw []byte) ([]byte, string, error) {
	reply, err := t.ingress.Hook(ctx, runnerID, provider, event, raw, engineagents.ChannelAPI)
	return reply.Body, reply.Ask, err
}

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
