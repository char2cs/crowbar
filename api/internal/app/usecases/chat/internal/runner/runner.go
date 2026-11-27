package runner

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type Runners struct {
	chats          agentchat.EventStore
	choices        termwait.Choices
	agents         engineagents.Agents
	term           seam.TerminalCommander
	ws             seam.WorkspaceReader
	runnerDirs     sync.Mutex
	home           func() (string, error)
	ancestorCwd    AncestorCwd
	ledger         *ledgerstore.Store
	daemonHost     string
	crowbarBin     string
	noAPITransport bool
	chatLock       ChatLock
	parks          *parks
	catalogs       *catalog.Runs
	minter         *agenttools.TokenMinter
	answers        *answerdesk.Desk
	apiConns       *apiConnRegistry
	attached       *attachRegistry
	surfaces       *surfaceRegistry

	conversations Conversations
	providers     Providers
	turns         Turns

	termWait termwait.Detector

	live       *liveTable
	phases     *phases
	snapshots  Snapshotter
	background *backgroundWork
	pause      func(ctx context.Context, d time.Duration) error
}

type Deps struct {
	Chats          agentchat.EventStore
	Choices        termwait.Choices
	Agents         engineagents.Agents
	Terminal       seam.TerminalCommander
	Workspace      seam.WorkspaceReader
	Home           func() (string, error)
	AncestorCwd    AncestorCwd
	Ledger         *ledgerstore.Store
	DaemonHost     string
	CrowbarBin     string
	NoAPITransport bool

	ChatLock ChatLock
	Minter   *agenttools.TokenMinter
	Answers  *answerdesk.Desk

	Conversations Conversations
	Providers     Providers
	Snapshots     Snapshotter
}

func New(d Deps) *Runners {
	return &Runners{
		chats:          d.Chats,
		choices:        d.Choices,
		agents:         d.Agents,
		term:           d.Terminal,
		ws:             d.Workspace,
		home:           d.Home,
		ancestorCwd:    d.AncestorCwd,
		ledger:         d.Ledger,
		daemonHost:     d.DaemonHost,
		crowbarBin:     d.CrowbarBin,
		noAPITransport: d.NoAPITransport,
		chatLock:       d.ChatLock,
		parks:          newParks(),
		catalogs:       catalog.New(),
		minter:         d.Minter,
		answers:        d.Answers,
		apiConns:       newAPIConnRegistry(),
		attached:       newAttachRegistry(),
		surfaces:       newSurfaceRegistry(),
		live:           newLiveTable(),
		phases:         newPhases(),
		snapshots:      d.Snapshots,
		background:     newBackgroundWork(),
		pause:          pauseFor,

		conversations: d.Conversations,
		providers:     d.Providers,
	}
}

func (rs *Runners) SetTurns(turns Turns) {
	rs.turns = turns
	screens, ok := rs.term.(termwait.Screens)
	if !ok {
		return
	}
	rs.termWait = termwait.New(termwait.Deps{
		Runners:  rs,
		Work:     rs,
		Choices:  rs.choices,
		Screens:  screens,
		Prompts:  turns,
		Notices:  turns,
		OnNotice: rs.failTurnOnNotice,
	})
}

func (rs *Runners) failTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	unlock, err := rs.chatLock.Lock(ctx, runner.CurrentChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: notice: hold the chat", "runner_id", runner.ID, "err", err)
		return
	}
	defer unlock()
	rs.turns.FailTurnOnNotice(ctx, runner, notice)
}

func composeContext(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n\n")
}
