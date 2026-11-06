// Package runner is the vendor CLI's lifecycle: start, switch provider, resume, stop, and deliver a Crowbar-authored
// prompt. Everything here is user-initiated and decides under the chat's hook lock (chatlock.go); the hook path must
// never call in through a locking door. A runner's process (PTY, serve connection or native view) is the sole authority
// on its liveness: a CLI is ended by killing it and letting its death record the exit.
package runner

import (
	"context"
	"log/slog"
	"strings"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Runners is the CLI lifecycle.
type Runners struct {
	chats agentchat.EventStore
	// choices are the questions each chat's CLI waits on, for the screen sweep.
	choices termwait.Choices
	agents  engineagents.Agents
	term    seam.TerminalCommander
	ws      seam.WorkspaceReader
	// ancestorCwd resolves a bubble's cwd through the tree usecase's ancestor
	// walk — see AncestorCwd's own doc for why this is declared here rather
	// than imported directly.
	ancestorCwd AncestorCwd
	// ledger is each chat's ledger: a prompt is a row there before any CLI sees it.
	ledger *ledgerstore.Store
	// daemonHost is the address the daemon bound, rendered into every callback's
	// --host so none derives one of its own.
	daemonHost string
	// crowbarBin is the crowbar binary the descriptors run (empty: the home's);
	// noAPITransport keeps every provider on hooks alone.
	crowbarBin     string
	noAPITransport bool
	// chatLock is the chat's lock a user-initiated path decides under; parks are
	// the waits it sits in outside it, which a stop cancels.
	chatLock ChatLock
	parks    *parks
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
	// attached is the terminal session of each runner CURRENTLY showing its native
	// TUI instead of its api connection; in memory, as it describes a live process.
	attached *attachRegistry
	// surfaces is the CURRENT surface each runner's chat is on — the
	// in-process mirror of domain.Chat.Surface. See surfaceRegistry.
	surfaces *surfaceRegistry

	conversations Conversations
	providers     Providers
	// turns is the hook ingress. Bound after construction: the two sides are built
	// together and neither can name the other first.
	turns Turns

	// termWait detects a CLI parked on a modal no hook reports. Nil when the terminal
	// seam cannot render a screen; every chat then reports the zero verdict.
	termWait termwait.Detector

	// live is the one CLI on each chat: in memory, like every process registry
	// here. Its durable facts are the ledger's runner rows.
	live *liveTable
	// phases is the lifecycle operation each chat is mid-way through — see
	// phase.go — and snapshots is where a change no aggregate event carries is
	// announced. snapshots is nil in tests that build a bare Runners.
	phases    *phases
	snapshots Snapshotter
	// background is the supervisor's own follow-up work; Shutdown ends it.
	background *backgroundWork
}

// Deps is everything the CLI lifecycle is built over. It is a struct and not an
// argument list because the shared in-flight state is built once, elsewhere,
// and an ordered list of pointers is a place to swap two of them silently.
type Deps struct {
	Chats agentchat.EventStore
	// Choices are the questions each chat's CLI waits on.
	Choices   termwait.Choices
	Agents    engineagents.Agents
	Terminal  seam.TerminalCommander
	Workspace seam.WorkspaceReader
	// AncestorCwd resolves a bubble's cwd through the tree usecase's ancestor
	// walk — see the AncestorCwd interface's own doc.
	AncestorCwd AncestorCwd
	Ledger      *ledgerstore.Store
	// DaemonHost is the address the daemon bound, rendered into the callbacks.
	DaemonHost string
	// CrowbarBin is the crowbar binary the descriptors run; empty is the one
	// installed under the home.
	CrowbarBin string
	// NoAPITransport runs every provider over hooks alone, its serve never forked.
	NoAPITransport bool

	// ChatLock is the lock the hook ingress records each chat's hooks under.
	ChatLock ChatLock
	Minter   *agenttools.TokenMinter
	Answers  *answerdesk.Desk

	Conversations Conversations
	Providers     Providers
	// Snapshots is the chat snapshot owner's announce side (optional).
	Snapshots Snapshotter
}

// New builds the CLI lifecycle. The hook-ingress port is bound separately, by
// SetTurns, because the two call each other.
func New(d Deps) *Runners {
	return &Runners{
		chats:          d.Chats,
		choices:        d.Choices,
		agents:         d.Agents,
		term:           d.Terminal,
		ws:             d.Workspace,
		ancestorCwd:    d.AncestorCwd,
		ledger:         d.Ledger,
		daemonHost:     d.DaemonHost,
		crowbarBin:     d.CrowbarBin,
		noAPITransport: d.NoAPITransport,
		chatLock:       d.ChatLock,
		parks:          newParks(),
		// Owned outright, so built here rather than handed in: the probe limiter is
		// named by nothing outside this package.
		catalogs:   catalog.New(),
		minter:     d.Minter,
		answers:    d.Answers,
		apiConns:   newAPIConnRegistry(),
		attached:   newAttachRegistry(),
		surfaces:   newSurfaceRegistry(),
		live:       newLiveTable(),
		phases:     newPhases(),
		snapshots:  d.Snapshots,
		background: newBackgroundWork(),

		conversations: d.Conversations,
		providers:     d.Providers,
	}
}

// SetTurns binds the hook ingress and builds the terminal-wait detector, here rather
// than in New because its screen classifiers belong to the ingress (bound by value).
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

// failTurnOnNotice records what a notice on runner's screen ends, under the
// chat's lock so no hook ends that work between its check and its record.
func (rs *Runners) failTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice) {
	unlock, err := rs.chatLock.Lock(ctx, runner.CurrentChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: notice: hold the chat", "runner_id", runner.ID, "err", err)
		return
	}
	defer unlock()
	rs.turns.FailTurnOnNotice(ctx, runner, notice)
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
