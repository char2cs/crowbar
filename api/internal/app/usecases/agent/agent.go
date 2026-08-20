// Package agent hosts the agentic-chat usecase. It owns two aggregates and the
// relationship between them: an AgentChat (a Crowbar thread, and the ledger on
// disk that is its only unique property) and an AgentRunner (one vendor CLI, in
// one PTY, that is currently pointed at one chat).
//
// The runner points at the chat. The chat never points back. That single arrow is
// what this package is built around: when a CLI moves between conversations — the
// user typing /clear or /resume INSIDE it — the move is ONE write to ONE aggregate
// (agentrunner.Move), and the chat being left is not written to at all. The
// previous model expressed the same move as EndSegment(A) + OpenSegment(B) across
// two aggregates with no transaction; it tore in half in production, committing the
// first and failing the second, and left chat A with no way back into it.
//
// Two rules govern everything here:
//
//   - PERSIST PLACEMENT, NEVER LIVENESS. Which chat and which conversation a runner
//     is on is Crowbar's own fact, so it is durable. Whether the CLI is ALIVE is the
//     PTY's fact and only the PTY's: "is this chat live?" is the query
//     LiveRunnerForChat (a row exists exactly while the process does), never a flag
//     we store and then have to keep true.
//
//   - RECONCILE, NEVER TRANSACT. By the time a hook reaches us the CLI has ALREADY
//     switched conversation. We cannot refuse it and cannot push it back, so the
//     hook path RECORDS what happened and must never fail on someone else's state.
//     Where two runners collide on one conversation, we act on the OTHER party
//     (evict it) rather than reject reality.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TerminalCommander is the terminal-engine seam the usecase spawns vendor CLIs
// through and tears them down with.
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

// WorkspaceReader resolves a workspace's Crowbar-managed identity and git
// worktree directory, plus the directory that holds its agentic chat state.
type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	// AgentChatsDir returns the directory holding the workspace's agentic chat
	// state — the per-chat handoff ledger and the per-spawn tmp dirs (the rendered
	// hook config; nothing else — no descriptor copies any credential into them,
	// and none may). It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// so plaintext ledgers never land on the user's filesystem. The worktree/Cwd is
	// unaffected — WorktreeDir still returns it unchanged.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}

// ChatLineage resolves a chat's CHAT ancestors in the Chats panel, nearest
// parent first, with any folders it is filed in filtered out.
//
// The spawn path needs it to answer one question: is this chat a THREAD, and of
// what? A thread is told its lineage at spawn (see threadContext) and reads
// those chats itself; nothing is copied for it. The port is narrow because that
// is the entire dependency — the tree the answer comes out of belongs to the
// Chats-panel usecase, and this package must not learn how it is shaped.
type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}

// Usecase is the agentic-chat engine: spawning vendor CLIs, ingesting their hooks
// through the context-move reducer, moving the runner that results, and appending
// the ledger. It does NOT broadcast: every agentchat.* and agentrunner.* event is
// fanned out to the WS hub by the repository layer's hub projections, so the single
// source of lifecycle frames is the event stream. The usecase's job ends at issuing
// the command that emits the event.
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

// New builds a Usecase over the two aggregates and the engine seams. registry is
// no longer a placement index — it holds only the per-spawn injected-context echo
// guard (see the agents engine's injection registry); every placement question is answered by the
// runner aggregate.
//
// minter and tools are the agent capability surface DispatchMCP serves. Both are
// optional: a Usecase built without them still runs chats, and DispatchMCP fails
// loudly instead of serving a tool-less agent. tools.Chats and tools.ChatLogs are
// deliberately NOT a caller's responsibility — the usecase IS the ChatRenamer AND
// the ChatLogReader (see ReadChatLog), so New fills both in and no caller can drop
// either tool by forgetting to hand the usecase back to itself.
//
// lineage is what makes a threaded chat read the chat it hangs off. A nil one is
// a chat panel with no tree behind it: every chat spawns as a standalone chat,
// which is exactly the behaviour of a daemon that predates threads and is the
// only shape a hand-assembled test Usecase needs. Production always wires it —
// the container builds it before this constructor runs, from the two stores the
// tree lives in.
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

// SpawnChat creates a fresh AgentChat AT THE PANEL ROOT and starts a runner on it,
// launching the provider's vendor CLI in a PTY. The returned runnerID is the
// crowbarSegmentID every hook from that CLI carries, and it is stable for the life
// of the process — including across every conversation move it makes.
//
// It mints the chat AFTER the CLI is live, and that ordering is only correct
// because this chat has nowhere to be placed. A chat born UNDER something is a
// different shape of create and takes a different route: agentchatfolder.CreateChat
// mints it, places it, and only then calls StartRunner, because a thread has to
// carry its parent edge before any CLI reads it. Keeping the two apart is
// deliberate — a plain new chat is created in exactly the order it always was.
//
// A create that FAILS creates nothing. The chat is written mid-spawn (recordRunner),
// so a spawn that dies after that point — a CLI that exits during startup, a runner
// row that will not commit — would otherwise answer "the spawn failed" while leaving
// the chat standing. discardSpawnedChat takes it back out, so the response and the
// state say the same thing.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	defer u.spawns.lock(chatID)()

	runnerID, err = u.spawnRunner(ctx, chatID, workspaceID, providerID, "", nil, nil, "", 0, false, "", true, "")
	if err != nil {
		return "", "", u.discardSpawnedChat(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

// MintChat creates a chat aggregate and starts NOTHING. It is the first half of a
// create whose second half is StartRunner, and it exists so the Chats-panel usecase
// can PLACE the new chat in the tree between the two.
//
// That gap is the entire reason for the split. A chat created under another chat is
// a thread of it, and a thread is told its lineage at spawn (threadContext) — so the
// parent edge has to be on the aggregate before any CLI starts, or the thread spends
// its whole first session believing it is a standalone chat. SpawnChat cannot open
// that gap: it necessarily writes the chat AFTER the process exists, because a pure
// command cannot fork one. That ordering is right for a chat with nowhere to be
// placed and impossible for a chat that has somewhere.
//
// A minted chat with no runner is not a broken state. It is the DORMANT chat the
// panel already models — liveness is the query LiveRunnerForChat, never a flag — so
// nothing has to be undone if the caller stops here. A caller that meant to start a
// CLI and could not is the one that has to decide what to do about that.
func (u *Usecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	chatID := uuid.NewString()
	created, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	u.work.set(chatID, created.Working)
	return chatID, nil
}

// StartRunner launches providerID's vendor CLI on a chat that ALREADY EXISTS and
// returns the runner now placed on it — the second half of the create MintChat
// opens, and the door every chat born inside the Chats tree comes up through.
//
// It resolves the workspace FROM the chat rather than taking one, so no caller can
// start a CLI against a workspace the chat does not belong to.
//
// Because the chat is on disk and already placed by the time this runs, the spawn
// resolves what that chat reads the ordinary way and injects it — which is what
// makes a thread's FIRST session know it is a thread. Nothing here special-cases
// that; it falls out of the chat existing first.
func (u *Usecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: start runner: chat: %w", err)
	}
	return u.spawnRunner(ctx, chatID, chat.WorkspaceID, providerID, "", nil, nil, "", 0, false, "", false, "")
}

// RenameChat sets a chat's title under user>agent>derived precedence:
//
//	source "derived": set only if the title is currently empty (first-prompt fallback).
//	source "agent":   set unless the title is user-locked (agent may upgrade a derived title).
//	source "user"/"": set unconditionally AND lock (a manual rename wins and sticks).
//
// The empty-title-is-a-no-op and derived-only-if-empty gates live here (the
// SetTitle command only enforces the locked-vs-user rule). A successful change
// emits a title_set event, which the hub projection fans out as the lifecycle
// frame — the usecase no longer broadcasts.
func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	if title == "" {
		return nil
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: rename chat: get: %w", err)
	}
	switch source {
	case "derived":
		if chat.Title != "" {
			return nil
		}
	case "agent":
		if chat.TitleLocked {
			return nil
		}
	default: // "user" / "" — manual rename wins and locks
		source = "user"
	}
	if _, err := u.chats.SetTitle(ctx, chatID, title, source); err != nil {
		return fmt.Errorf("agent: rename chat: save: %w", err)
	}
	return nil
}

// RenameByRunner resolves runnerID to the chat it is placed on RIGHT NOW and
// applies RenameChat to it — the same runnerID → runner → CurrentChatID
// resolution IngestHook uses for every hook (see its doc comment). It is what
// the agent's own set_chat_title tool calls, and its ONLY caller: the agent is
// never told a chat id, so a CLI that has since moved to a different chat (a
// /clear or /resume issued inside it) can never rename the chat it used to be
// on.
//
// A displaced runner (CurrentChatID == "" — Crowbar has taken it off its chat
// and is killing it, but the process has not yet died) has nowhere to write
// the title and is a silent no-op, mirroring handleTurn's identical guard on
// the hook path: there is nothing actionable a dying CLI could do with an
// error here.
func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("agent: rename by runner: runner: %w", err)
	}
	if runner.CurrentChatID == "" {
		return nil
	}
	return u.RenameChat(ctx, runner.CurrentChatID, title, source)
}

// ReadChatLog returns chatID's whole ledger as turns for the get_chat_log tool.
// It is this usecase's implementation of agenttools.ChatLogReader — see New's
// doc comment for why tools.ChatLogs is filled in there rather than by a caller:
// the ledger lives behind this package's own storage, so the usecase IS the
// reader, the same way it is its own ChatRenamer.
//
// Turns, not rendered text: get_chat_log CAPS what it hands a model and states
// how many turns it left out, and a count taken from re-split text would be
// wrong (a turn's body contains blank lines, which is what the rendering
// separates turns with). Rendering the window it keeps is the tool's job; this
// method's is to report what there is. The whole ledger is returned rather than
// a pre-cut window because the cap and its wording belong with the other two
// caps on the tool surface, not spread across the usecases behind them.
//
// Scope is NOT checked here: get_chat_log's tool handler already confirmed
// chatID's workspace is in the caller's visible set before this is ever
// called (a chat id is not itself an authorization), so this method trusts
// its caller the same way openLedger's other callers do.
//
// An empty ledger — a chat that has not spoken yet — is returned as no turns,
// not an error. Turning that into agenttools.NoChatTurnsText is the TOOL's job
// (getChatLog), not this method's: get_chat_log is the only caller today, and
// duplicating that normalization here would just be a second place the exact
// wording could drift from the tool's.
func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read chat log: chat: %w", err)
	}
	turns, err := u.chatTurns(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read chat log: turns: %w", err)
	}
	out := make([]agenttools.ChatTurn, 0, len(turns))
	for _, t := range turns {
		out = append(out, agenttools.ChatTurn{Speaker: t.Speaker(), Body: t.Text})
	}
	return out, nil
}

// DispatchMCP runs one MCP message on behalf of the runner named by runnerID.
//
// It is the ONLY entry point to the agent tool surface, which is what keeps
// authorization in one place: the relay process that carries these bytes never
// decides anything, and the ToolSet is constructed around this caller's
// credentials so no tool handler is reachable without a successful Resolve.
//
// The ToolSet is built PER CALL rather than cached per runner because the
// credentials are what the tools close over — a cached set would outlive the
// runner it was minted for.
//
// The bool reports whether a reply should be sent: a JSON-RPC notification is
// answered with silence.
func (u *Usecase) DispatchMCP(
	ctx context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	if u.minter == nil || u.tools.Resolver == nil {
		return nil, false, fmt.Errorf("agent: dispatch mcp: tool surface not configured")
	}
	tools := agenttools.NewToolSet(u.tools, runnerID, token)
	server := enginemcp.NewServer("crowbar", metadata.GetVersion(), tools)
	out, send := server.Handle(ctx, message)
	return out, send, nil
}

// PurgeChat hard-deletes chatID via asynx Forget: the aggregate's event log AND
// its read-model row are erased, so a subsequent GetChat/ListChats genuinely
// reports not found. It then kills the CLI that was pointed at the chat, drops the
// chat's conversation history, and reaps its on-disk footprint.
//
// A runner may still be pointed at the chat, and a runner whose chat no longer exists is
// a runner with nowhere to write. It is DISPLACED (taken off the chat — a placement fact,
// which is ours) and then killed (a request, not an assertion). We do NOT delete its row
// from the read model: that would be Crowbar asserting a LIVENESS fact it does not own —
// the exact dual-authority mistake this refactor deletes. The row goes when the PTY does
// (TerminateGraceful → the engine's onExit → Exit → the projection drops it).
//
// ORDER: Forget the chat FIRST, kill the CLI SECOND. This is deliberate and it is
// the opposite of what the old code did.
//
//	The teardown fires reconcileRunnerExit asynchronously, off the terminal engine's
//	reap goroutine, and that path writes to the chat (it closes a turn the dead CLI
//	left open). asynx projections are async, so a chat command that COMMITS before the
//	Forget can have its read-model Save land AFTER Forget's row-delete — resurrecting
//	the deleted chat as a zombie row. That bug was found in live daemon testing, and
//	the old code fought it with an in-memory registry unbind (a synchronous guard the
//	teardown checked first). That registry is gone, so the guard has to be structural:
//	once the aggregate is Forgotten its event log is erased, so EVERY subsequent chat
//	command fails Validate (current == nil) and emits nothing at all. Forgetting first
//	makes the zombie unrepresentable rather than merely unlikely.
//
//	The window it opens instead is closed by IngestHook's chat-existence guards, which
//	cover BOTH hook kinds: a turn hook whose chat is gone is dropped, and so is a
//	conversation ANNOUNCEMENT (which would otherwise write a conversation-history row
//	against a chat that has just been deleted — a dangling /resume target). Either way
//	the CLI is killed again: it has nowhere left to write.
//
// Everything after the Forget is BEST-EFFORT: a terminate failure, a history-drop
// failure or a filesystem failure is logged and the purge still reports success.
// Wedging a delete the user asked for (leaving a chat they can never remove) is a
// worse outcome than an orphaned PTY or a leftover directory.
func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason the spawn paths take it: a delete racing a
	// switch would otherwise Start a CLI onto a chat that has just been Forgotten. That
	// self-heals (the runner's first hook finds no chat and retires it), but only after
	// spawning a real process and leaving its tmp dir behind. Serialising is a line.
	defer u.spawns.lock(chatID)()

	return u.purgeChatLocked(ctx, chatID)
}

// purgeChatLocked is PurgeChat's whole body with the gate ALREADY HELD — the same
// split switchProviderLocked makes, and for the same reason: the gate is not
// re-entrant, so a path that already holds a chat's gate cannot reach the delete
// through its exported door.
//
// Its second caller is a create that failed. SpawnChat holds the gate across the
// whole spawn (a fresh uuid, so uncontended by construction) and has to take the
// half-made chat back out from INSIDE that section. Releasing the gate first and
// then calling PurgeChat would work mechanically, but it would publish the very
// state the create is disowning: for the width of that window the chat exists,
// unlocked, and any other gated path — a switch, a resume, a delete — could take
// it and act on a chat whose creator has already decided it does not exist.
// Sharing the locked body keeps "created and refused" atomic under the one lock
// that orders this chat, which is what a caller told 424 is entitled to assume.
func (u *Usecase) purgeChatLocked(
	ctx context.Context,
	chatID string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: purge chat: get: %w", err)
	}
	if err := u.chats.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agent: purge chat: forget: %w", err)
	}
	u.retireChatRunners(ctx, chatID)

	// Drop the conversation record and its telemetry. The record outlives the
	// process and nothing else removes it, so a hard delete that skipped it would
	// leave the chat's plaintext readable after the user asked for it to be gone.
	if err := u.activity.Forget(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation record (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
	u.telemetry.forget(chatID)

	// Drop the chat's conversation history. It is append-only and outlives the
	// process, so nothing else ever removes it — and a conversation still pointing
	// at a hard-deleted chat is a live trap: a later /resume of that session id would
	// resolve (ChatForSession) to a chat that no longer exists.
	if err := u.runners.ForgetChat(ctx, chatID); err != nil {
		slog.ErrorContext(ctx, "agent: purge chat: forget conversation history (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}

	// Reap the chat's on-disk footprint now the aggregate is gone. The conversation
	// itself no longer lives here — it is in the record dropped above — but a chat
	// directory may still hold whatever a spawn left in it.
	//
	// NOT the runner's tmp dir, which no longer lives under the chat (worktreepath.RunnerDir)
	// and is no business of the chat's: it belongs to the PROCESS lifecycle, and the process
	// we have just SIGTERM'd is still alive and still reading it. Removing a live CLI's
	// config out from under it was only ever an accident of the old layout. It goes when the
	// PTY does (onExit), or at the next boot if the daemon died first.
	//
	// The removal is routed through RemoveUnderHome, which re-asserts the target is strictly
	// under crowbar home, so even a poisoned chats dir can never reach the user's real
	// repository.
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve chats dir for reap (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: purge chat: resolve home for reap guard (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return nil
	}
	RemoveUnderHome(ctx, home, filepath.Join(chatsDir, chatID))
	return nil
}

// discardSpawnedChat takes a chat back out when the create that minted it failed
// after it was already written, and hands back the failure that caused it. It is
// SpawnChat's counterpart to agentchatfolder.CreateChat's discard, and it is
// called with the chat's spawn gate already held (hence purgeChatLocked).
//
// A refused create that leaves a chat behind is the record contradicting its own
// response: the API says the spawn failed, the state says a chat exists, and the
// user is left holding a chat they never successfully made — the same defect class
// as a prompt recorded `answered` whose bytes never reached the CLI, and a spinner
// still spinning after the CLI is done. Whichever half the client believes, the
// other half is lying to it.
//
// It is a no-op when the spawn failed BEFORE the chat was written (a disabled
// provider, an unresolvable descriptor, a CLI that is not installed): there is
// nothing to take back, so a missing chat is the expected answer and not worth a
// log line.
//
// The purge itself is best-effort and NEVER replaces the cause. What the user
// asked for was a chat, and that is what failed, so that is what they are told; a
// purge that fails on top of it leaves a dormant chat, which is visible in the
// panel and deletable by hand — a far better outcome than reporting an error other
// than the one that actually happened.
func (u *Usecase) discardSpawnedChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.purgeChatLocked(ctx, chatID); err != nil && !errors.Is(err, agentchat.ErrNotFound) {
		slog.WarnContext(ctx, "agent: discard chat of a refused spawn (best-effort, reporting the spawn failure)",
			"chat_id", chatID, "err", err)
	}
	return cause
}

// retireChatRunners takes EVERY vendor CLI on chatID off that chat and kills it. A dormant
// chat is a no-op — that absence is the answer, not an error.
//
// The plural read, not the single-row one: this is a DELETE, and a delete is precisely where
// you want everyone gone. If the invariant were transiently broken (two placements racing
// this chat), killing "the" runner would leave the other one alive and running against a
// chat that is about to be Forgotten — an invisible CLI, writing into nothing.
//
// Best-effort by contract: every caller is a delete the user asked for, and none of them may
// be wedged by a failure here.
func (u *Usecase) retireChatRunners(
	ctx context.Context,
	chatID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: look up runners on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	for _, r := range placed {
		u.retire(ctx, r)
	}
}

// retire takes a runner off the chat it is on and asks its process to quit — in that
// order, and the order is the point.
//
// DISPLACE FIRST. It cannot fail on anyone else's state, and it records the one thing we
// actually know: this CLI is no longer on that chat. It says NOTHING about whether the
// process is alive, so it is not a second opinion on liveness — it is a placement fact,
// and placement is ours alone. Recording it immediately is what makes "one runner per
// chat" true at every instant: a SIGTERM'd CLI does not die synchronously, and until it
// falls over it would otherwise still be pointed at a chat somebody else now owns —
// where it could write turns into that chat's ledger, and where a read could hand it out
// as the chat's live runner (attaching a pane to a corpse).
//
// KILL SECOND, best-effort. TerminateGraceful is a request; the PTY decides. If it fails
// the CLI stays alive — but it is already unplaced, so its hooks resolve to no chat and
// are dropped, and its death (whenever it comes) still carries its row away. A failure
// here therefore leaks a process, never a write.
func (u *Usecase) retire(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	if err := u.displace(ctx, runner); err != nil {
		// Best-effort: the runner is still on its chat, so its own exit will close any turn
		// it leaves open. We still kill it.
		slog.ErrorContext(ctx, "agent: retire runner: displace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
	}
	if err := u.term.TerminateGraceful(ctx, runner.TerminalSession); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: retire runner: terminate (best-effort, continuing)",
			"runner_id", runner.ID, "terminal_session_id", runner.TerminalSession, "err", err)
	}
}

// displace records that a runner is no longer placed on any chat, and then closes any turn
// it was leaving open on the chat it just left.
//
// Closing that turn HERE is not an opinion about liveness, and it has to be here. Turn
// state is normally repaired by the dying CLI's own exit (reconcileRunnerExit), which finds
// the chat through runner.CurrentChatID — the very field this command erases. Without this,
// a switch that aborts after the outgoing CLI was quit (an unknown target provider, a
// failed spawn) leaves the chat with no runner and Working=true FOREVER: the chat row
// spins, and the workspace's whole overlay spins with it, until the user resumes that chat
// and completes another turn. Once a runner is displaced, no hook of its can ever reach
// that chat again — so "nobody is working on this chat" is simply the last true thing we
// can say about it.
//
// An already-EXITED runner is not an error: a CLI quitting on its own moments before we
// went to take it off a chat is an ordinary, benign race, and the command tells us so with
// ErrValidation. Its exit has already cleared everything this would have.
func (u *Usecase) displace(
	ctx context.Context,
	runner domain.AgentRunner,
) error {
	vacated := runner.CurrentChatID

	if _, err := u.runners.Displace(ctx, runner.ID); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: displace runner: it had already exited (benign)",
				"runner_id", runner.ID, "chat_id", vacated)
			u.turns.complete(runner.ID)
			return nil
		}
		return fmt.Errorf("agent: displace runner: %w", err)
	}
	// Once displacement commits this runner can no longer deliver a hook into the
	// vacated chat. Resolve any pending React dispatch from the hook-derived ledger,
	// or mark it safely retryable when no matching user turn exists.
	u.reconcilePromptRunnerDeparture(ctx, runner, vacated)

	// A displaced runner is on NO chat, so handleTurn drops every hook it has left —
	// including the turn_stop that would have ended a turn it is still mid-way through.
	// Nothing will ever close that turn where it stood, so anybody waiting on it is
	// released here or waits for their context to die (turnWaits.complete).
	u.turns.complete(runner.ID)
	u.closeAbandonedTurn(ctx, vacated)
	return nil
}

// closeAbandonedTurn stops a turn on chatID when NOTHING is running on that chat any more —
// the turn's CLI is gone (dead, or displaced and dying) and no successor has taken the chat
// over, so the turn_stop hook that would have closed it is never coming.
//
// The guard here is the one thing only THIS layer knows: "is there still someone whose turn
// this is?" — a chat somebody else is now on (a provider switch spawns the incoming CLI
// before the outgoing one has fallen over) keeps ITS turn. An empty chatID means NOWHERE — a
// runner that is already displaced — and nowhere is never written to.
//
// IT DOES NOT ASK WHETHER THE CHAT IS WORKING, and that omission is the fix for a wedge that
// shipped. It used to, reading domain.AgentChat.Working via GetChat and returning early when
// it said idle — but GetChat serves the READ MODEL, which an ASYNCHRONOUS projection folds,
// while the turn that opened is durable in the event log the instant StartTurn returns. So
// the guard was a read-then-act against state that lags the truth it decides on: the outgoing
// CLI's last prompt lands microseconds before the displace, the projection has not caught up,
// the guard reads a stale false, and the turn is closed by NOTHING — the chat spins forever,
// and the workspace's whole overlay spins with it. It is not self-healing: only the user
// resuming that chat and completing another turn ever clears it. Measured in test at roughly
// 1 displace in 7 on an idle machine — the production rate is unknown and depends on how the
// projection is scheduled against the teardown, but one loss is one chat wedged for good.
//
// The question was not wrong, only asked in the wrong place. It is now asked inside the
// command (commands.StopTurn.Validate), which asynx evaluates against the authoritative fold
// of the event log and commits atomically with the append at that same version — so an idle
// chat still emits no event, and a turn opening concurrently collides on the version and is
// re-validated by the OCC retry. There is no window left to lose, because there is no longer
// a gap between the question and the act.
//
// ErrValidation is therefore ordinary and expected: it is the command saying "there is
// nothing here to close", or "this chat is gone" (PurgeChat Forgets the chat before retiring
// its runners) — the same benign signal displace already reads it as for an already-exited
// runner.
//
// KNOWN ASYMMETRY, deliberately not fixed. On the SWITCH path this closes the outgoing CLI's
// turn (the chat is momentarily empty when we run). On the EVICTION path it does not: the
// mover is already on the chat, so the first guard sends us home — and the evictee's own
// turn_stop is now dropped, because it is unplaced. So evicting a MID-TURN incumbent leaves
// the chat spinning until the MOVER finishes a turn of its own.
//
// It self-heals, it is pre-existing in shape, and the alternative is worse: closing that turn
// would mean asserting "the evicted CLI is not working" about a process that is still alive
// and that we have merely asked to leave — a liveness claim, which is precisely what this
// model refuses to make. A visible spinner that resolves beats an invented fact.
func (u *Usecase) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
) {
	if chatID == "" {
		return
	}
	if _, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return // someone else is on this chat now: its turn is not ours to close
	}
	// AbandonTurn, not StopTurn: the CLI is GONE, so it will never restate the level of
	// async work it last reported outstanding, and a plain StopTurn would leave that
	// number standing — Working is folded from the turn OR that level, so the chat would
	// spin forever on work nothing is doing. That is the same wedge this function exists
	// to prevent, one field over. Nothing a dead CLI announced survives it.
	//
	// This is the ONLY thing standing between a killed CLI and a permanently-spinning
	// chat: measured against claude 2.1.212, a SIGKILL mid-background-work sends no
	// SessionEnd and no final Stop — the last word is a turn_stop reporting work still
	// running, and in an event-sourced aggregate that word outlives the restart.
	// The conversation record is closed FIRST and unconditionally. A dead CLI's
	// turn is open there too, holding whatever tool calls it had in flight — and
	// those render as running for as long as the record says so, which is forever.
	// The next turn's activity would attach to that stale open turn as well.
	if err := u.activity.Abandon(ctx, chatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: abandon conversation record",
			"chat_id", chatID, "err", err)
	}

	abandoned, err := u.chats.AbandonTurn(ctx, chatID, time.Now())
	if err == nil {
		u.work.set(chatID, abandoned.Working)
		return
	}
	if errors.Is(err, asynxModels.ErrValidation) {
		// The command's authoritative fold says there is nothing to abandon.
		u.work.set(chatID, false)
		return
	}
	slog.WarnContext(ctx, "agent: close abandoned turn: abandon turn", "chat_id", chatID, "err", err)
}

// ReconcileRunnersOnBoot runs ONCE at startup. A PTY does not survive a daemon restart, so
// every runner whose terminal session is not backed by a live process is dead — and NOTHING
// recorded those deaths, because the only thing that ever records one is an exit callback
// living in the process that just went away. What the daemon comes back to is a durable
// sqlite table (agent_runners is never truncated) full of live rows describing CLIs that no
// longer exist.
//
// Leaving them there is not a cosmetic staleness. A stale row is indistinguishable from a
// running CLI to every read in this package, so it BRICKS the chat it names: ResumeChat asks
// LiveRunnerForChat first, is handed the corpse, and returns it as a no-op — the Resume
// button silently does nothing, forever, and the pane attaches to a dead terminal session.
//
// This is the ONE place liveness is reconciled, and it reconciles against the PTY — the
// single authority. It replaces the old boot reactor that ended SEGMENTS, which maintained a
// SECOND opinion about liveness and could therefore disagree with reality (observed: a
// segment reading "ended" while its CLI was demonstrably still running).
//
// It is driven off ALL live runners, not off the chats. A runner Displace has taken off its
// chat is placed NOWHERE, so no chat can reach it — and if the kill that followed the
// Displace failed, its row is otherwise immortal. Those rows are precisely the ones nothing
// else will ever clean up, and a chat-driven sweep would walk straight past them.
//
// Best-effort per runner: one runner's failure is logged and the rest are still reconciled.
// A boot hook that gives up halfway leaves an arbitrary suffix of chats bricked.
func (u *Usecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	runners, err := u.runners.AllLive(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: list runners: %w", err)
	}
	for _, r := range runners {
		// SessionLive is the seam that asks "is this PROCESS alive", NOT the engine's
		// SessionExists, which is also true for a PTY-less suspended placeholder — a session
		// whose process is already dead and whose only remaining substance is scrollback on
		// disk. Restoring those placeholders is the boot step immediately before this one, so
		// asking the registry "do you know this id?" here would answer yes for every single
		// dead agent CLI and reconcile NOTHING. That is the exact mistake that let a
		// restart-orphaned chat go on advertising a live agent.
		if u.term.SessionLive(ctx, r.TerminalSession) {
			continue
		}
		// Read the placement BEFORE the Exit: it is the chat whose turn this CLI abandoned.
		abandoned := r.CurrentChatID

		// The tmp reap below is intentionally gated on a SUCCESSFUL Exit, even though the
		// PTY is already known dead at this point and the dir could be reaped regardless.
		// Coupling them keeps the row and its tmp dir moving together: if Exit fails, the
		// row is still recorded live, so this same runner reappears in AllLive on the next
		// boot and both the Exit and the reap are retried then. Reaping now while Exit's
		// error leaves the row live would desync the two — a "live" row with its config
		// already gone — for no gain, since the dir holds no credential and an extra boot
		// of staleness is the same benign wait the surrounding best-effort loop already
		// accepts everywhere else.
		if _, err := u.runners.Exit(ctx, r.ID, time.Now()); err != nil {
			slog.ErrorContext(ctx, "agent: boot reconcile: exit dead runner (best-effort, continuing)",
				"runner_id", r.ID, "terminal_session_id", r.TerminalSession, "err", err)
			continue
		}
		u.reconcilePromptRunnerDeparture(ctx, r, r.CurrentChatID)
		u.reapCrashOrphanRunnerTmp(ctx, r)

		// Close the turn it died in the middle of. Turn state has never been durable truth
		// (domain.AgentChat.Working is documented as reconciled, not authoritative — a CLI
		// that dies mid-turn never sends the turn_stop hook that would close it), so
		// repairing it asserts nothing about any process. Without this, a chat that was
		// working when the daemon died comes back spinning and spins forever, and the
		// workspace's whole overlay spins with it.
		//
		// The Exit above is SendWait, so the "is anyone still on this chat" read inside can
		// no longer see the runner we have just reaped.
		u.closeAbandonedTurn(ctx, abandoned)
	}
	if err := u.reconcilePromptJournalsOnBoot(ctx); err != nil {
		return err
	}
	return nil
}

// reapCrashOrphanRunnerTmp removes the per-spawn tmp dir of a runner whose PTY died with the
// daemon. On a clean exit the onExit callback rm's it; a crash is exactly the case where that
// callback never ran, so these dirs are the one orphan class that accumulates forever.
//
// They hold the rendered hook config and nothing else — no descriptor copies a credential
// into them, and none may (the engine's only inject verbs are set_env, write_file and
// pass_arg; there is no copy_file). So this is hygiene, not a leak.
//
// The path is derived from the RUNNER (id + provider + workspace), never from its chat: the
// runners this most needs to reap are the DISPLACED ones, and Displace erases CurrentChatID.
// See worktreepath.RunnerDir — a tmp dir that could only be found through a pointer the
// system is free to erase is a tmp dir that cannot be reaped.
//
// Best-effort: the workspace may be gone, and a leftover directory is a far smaller harm than
// a boot hook that aborts. RemoveUnderHome re-asserts the target is strictly under crowbar
// home, so even a poisoned chats dir can never make this reach the user's real filesystem.
func (u *Usecase) reapCrashOrphanRunnerTmp(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	chatsDir, err := u.ws.AgentChatsDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: chats dir (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: home (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	RemoveUnderHome(ctx, home, worktreepath.RunnerDir(chatsDir, runner.ID, runner.ProviderID))
}

// deriveTitle turns a user prompt into a short chat title: the first non-empty
// line, trimmed, capped to 60 runes.
func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}

// mintRunnerToken issues the credential runnerID's in-PTY `crowbar mcp` relay
// authenticates with.
//
// A Usecase built WITHOUT a minter is legal (the tool surface is optional — see
// New), and such a runner must still spawn: chats are the product, tools are an
// addition to it. It carries an empty token instead, which Verify rejects
// unconditionally, so the tool surface fails closed rather than the chat failing to
// open.
func (u *Usecase) mintRunnerToken(runnerID string) string {
	if u.minter == nil {
		return ""
	}
	return u.minter.Mint(runnerID)
}

// spawnRunner is the single spawn seam: it launches providerID's vendor CLI in a
// PTY and records the runner that results, pointed at chatID. Every path that
// starts a CLI goes through it — SpawnChat (create=true, minting the chat too),
// SwitchProvider and ResumeChat (create=false, on a chat that already exists).
//
// It does ALL the IO first — resolve the descriptor, render the per-spawn tmp dir
// and hook config, build the spawn plan, launch the PTY — and only THEN issues the
// persistence commands. A pure command cannot spawn a process, so the CLI is
// necessarily live before the runner can be recorded; if either command fails, the
// just-spawned CLI is torn down so no orphan process leaks.
//
// conversation is the already-wrapped handoff document (the full ledger for a
// provider new to this chat, the gap only for one resumed into its own session, ""
// for a brand-new chat). It is composed with the capability preamble into the ONE
// {context} document the descriptor injects, because a provider may only have a
// single such channel — codex delivers both through the same
// developer_instructions key, so injecting them separately would have the second
// silently overwrite the first.
//
// resuming selects WHICH descriptor channel carries that document: ContextInject
// for a fresh session, ResumeContextInject for one resumed via session.resume. The
// distinction is real, not cosmetic — a resumed codex ignores every config channel
// and can only be reached through a user message (see codex.yaml) — and it is the
// descriptor, never this code, that knows what each provider needs.
func (u *Usecase) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	preallocatedRunnerID string,
	extraSteps []engineagents.InjectStep,
	finalSteps []engineagents.InjectStep,
	conversation string,
	gapTurns int,
	resuming bool,
	launchSessionID string,
	create bool,
	promptMessage string,
) (string, error) {
	pre, err := u.spawnPreflight(ctx, chatID, providerID, create)
	if err != nil {
		return "", err
	}
	threads, sel := pre.threads, pre.selection
	runnerID := newRunnerID(preallocatedRunnerID)

	paths, err := u.spawnPaths(ctx, workspaceID, runnerID, providerID)
	if err != nil {
		return "", err
	}
	crowbarHome, projectID, repoID := paths.crowbarHome, paths.projectID, paths.repoID
	worktree, tmpDir := paths.worktree, paths.tmpDir

	descriptor, err := u.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}
	// The tool surface is switched off by rendering a descriptor that does not
	// declare one, rather than by filtering steps at the injection site: WHERE those
	// steps land is the descriptor's business (claude's --mcp-config is variadic and
	// needs the --settings pair immediately behind it), and this function has no
	// business knowing that.
	descriptor = descriptor.WithTools(pre.mcpOn)

	tctx, inject := u.renderSpawnContext(spawnContext{
		chatID:        chatID,
		workspaceID:   workspaceID,
		providerID:    providerID,
		projectID:     projectID,
		repoID:        repoID,
		runnerID:      runnerID,
		tmpDir:        tmpDir,
		worktree:      worktree,
		crowbarHome:   crowbarHome,
		threads:       threads,
		conversation:  conversation,
		promptMessage: promptMessage,
		gapTurns:      gapTurns,
		resuming:      resuming,
		selection:     sel,
	})

	steps := append([]engineagents.InjectStep{}, extraSteps...)
	// The chat's model/effort choice, rendered by the descriptor that declares it.
	// An unset choice, or a provider declaring no such block, contributes an EMPTY
	// slice — so this line is the whole of the feature's cost on a spawn that is
	// not using it, and the argv is byte-identical to one rendered before it
	// existed.
	steps = append(steps, descriptor.SelectionSteps(sel)...)
	if inject {
		steps = append(steps, descriptor.ContextSteps(resuming)...)
	}
	// Positional user prompts are final by contract. In particular Claude's
	// variadic --mcp-config must already have been terminated by later options,
	// and Codex's resume subcommand/id must precede the message.
	steps = append(steps, finalSteps...)

	// Register the injected document BEFORE the CLI can run: a provider whose only
	// resume channel is a user message (codex) fires its user-prompt hook with this
	// exact text the moment it starts, and that echo must never be recorded as a
	// ledger turn — that is what made handoffs nest inside themselves.
	//
	// Only when something was ACTUALLY injected. contextInject is the sole channel for
	// both the document and the pointer, so an un-injected spawn has nothing that can
	// echo — and since the capability preamble makes tctx.Context non-empty on every
	// spawn, registering unconditionally would leave a guard behind for text no CLI
	// was ever given.
	if inject {
		u.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		// The injected-context entry above was registered before the CLI could exist, and
		// only reconcileRunnerExit (via the onExit callback) ever forgets it — a callback
		// that never fires when the CLI never goes live. Forget it here, or every failed
		// spawn leaks one handoff-sized string until the daemon restarts.
		u.agents.ForgetRunner(runnerID)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}

	// binpath.Resolve, never the bare descriptor cmd: the PTY exec's argv[0] through
	// exec.Command, which resolves a bare name against the DAEMON's PATH — plan.Env is
	// ignored for the lookup. A launchd-started .app daemon has a minimal PATH that
	// misses ~/.local/bin, where claude and codex install, so a bare name made every
	// spawn die with "executable file not found in $PATH". An unresolvable cmd passes
	// through unchanged, preserving that error for a CLI that genuinely is not installed.
	argv := append([]string{plan.Executable}, plan.Argv...)
	termSessID, err := u.forkCLI(ctx, forkRequest{
		runnerID:    runnerID,
		providerID:  providerID,
		workspaceID: workspaceID,
		worktree:    worktree,
		crowbarHome: crowbarHome,
		tmpDir:      tmpDir,
		argv:        argv,
		env:         plan.Env,
	})
	if err != nil {
		return "", err
	}

	if err := u.recordRunner(
		ctx, chatID, workspaceID, providerID, runnerID, termSessID, launchSessionID, sel, create,
	); err != nil {
		u.pendingHooks.discard(runnerID)
		u.agents.ForgetRunner(runnerID)
		return "", err
	}
	// Keep the barrier installed throughout replay. A hook arriving while an
	// earlier buffered hook is being applied joins the next batch, so it cannot
	// overtake session_start or user_prompt on the normal persisted-runner path.
	//
	// exitedDuringStartup means exactly one thing, and it is narrower than its name:
	// the PTY died BEFORE the runner row committed, so the exit callback had no row
	// to reconcile against and left the fact here instead. It is a RACE the CLI has
	// to lose to be caught — one that dies 50ms later wins it, gets a 201, and
	// reconciles through onRunnerExit into an ordinary DORMANT chat.
	//
	// So 424 is not a guarantee that a chat handed back has a living CLI behind it,
	// and nothing may be built on reading it that way. Both outcomes are honest and
	// both are visible — a refusal that names the dependency, or a chat the panel
	// already draws as dormant with no runner on it — and the only way to collapse
	// them into one deterministic answer would be to wait or probe after EVERY
	// spawn, paying latency on the common path to close a corner. That is the trade,
	// deliberately made in this direction; it is not an oversight to be tightened.
	exitedDuringStartup := u.pendingHooks.finish(runnerID, func(hook pendingRunnerHook) {
		u.replayStartupHook(runnerID, hook)
	})
	if exitedDuringStartup {
		// onExit could not reconcile before the row existed. Now it does, after
		// every hook the provider emitted before dying has had its ordered chance
		// to update the ledger and prompt journal.
		u.reconcileRunnerExit(context.Background(), runnerID)
		return "", ErrProviderExitedDuringStartup
	}
	return runnerID, nil
}

// replayStartupHook applies one hook a provider fired before its runner row
// existed.
//
// Best-effort by design: the process is already live and the user is already
// talking to it, so a hook that cannot be applied is logged and the rest of the
// batch continues. Its delivery record is completed only AFTER the effects land,
// which is what keeps a redelivery idempotent rather than lost.
func (u *Usecase) replayStartupHook(
	runnerID string,
	hook pendingRunnerHook,
) {
	replayCtx := context.Background()
	if hook.deliveryID != "" {
		replayCtx = context.WithValue(replayCtx, hookDeliveryContextKey{}, hook.deliveryID)
	}
	if err := u.ingestHookNow(
		replayCtx, runnerID, hook.provider, hook.canonicalEvent, hook.rawPayload,
	); err != nil {
		slog.Error("agent: replay startup hook (best-effort, continuing)",
			"runner_id", runnerID, "event", hook.canonicalEvent, "err", err)
		return
	}
	if hook.deliveryID == "" {
		return
	}
	if err := u.hookDeliveries.complete(
		hook.deliveryDir, hook.deliveryID, hook.deliveryHash, time.Now(),
	); err != nil {
		slog.Error("agent: persist replayed startup hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", hook.deliveryID, "err", err)
	}
}

// newRunnerID returns the id this spawn's runner will carry.
//
// It IS the crowbarSegmentID passed to every hook, minted before the process
// exists — so a hook fired the instant the CLI comes up can always name its
// runner. It is stable for the whole life of that process, including across every
// conversation move: that stability is what makes a move one write instead of a
// delete-here/insert-there.
func newRunnerID(
	preallocated string,
) string {
	if preallocated != "" {
		return preallocated
	}
	return uuid.NewString()
}

// spawnPaths resolves every filesystem fact a spawn needs and creates the
// runner's own directory.
type spawnPaths struct {
	crowbarHome string
	projectID   string
	repoID      string
	worktree    string
	tmpDir      string
}

func (u *Usecase) spawnPaths(
	ctx context.Context,
	workspaceID, runnerID, providerID string,
) (spawnPaths, error) {
	crowbarHome, projectID, repoID, worktree, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: worktree dir: %w", err)
	}

	// The chats dir is resolved separately from the worktree/Cwd: for a home-kind
	// (adopted checkout) workspace the worktree is the user's REAL dir outside home,
	// so chat state (this tmp dir, the ledger) reroots under crowbar home while the
	// CLI still runs with Cwd = worktree.
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

	// Under the workspace's chats dir (always beneath crowbar home), keyed by the RUNNER —
	// id + provider — and NOT by the chat. The chat pointer is erasable (Displace clears it
	// while the process still runs), and a dir that can only be found through an erasable
	// pointer can never be reaped; see worktreepath.RunnerDir. This path is derivable from
	// a bare runner row forever, which is what lets BOTH removers find it: onExit below on a
	// clean death, and boot reconciliation when the daemon died before onExit could run.
	//
	// It holds the rendered hook config the CLI is pointed at, and must survive for the
	// whole life of that CLI — so it is removed on PTY death, never eagerly after spawn.
	//
	// It holds no secret: a provider owns its own credentials and Crowbar never copies
	// them anywhere (that rule is why CODEX_HOME is not ours, and why codex's sessions
	// survive a switch). Nothing in the descriptors puts a credential here, and nothing
	// may.
	tmpDir := worktreepath.RunnerDir(chatsDir, runnerID, providerID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: mkdir tmp: %w", err)
	}
	return spawnPaths{
		crowbarHome: crowbarHome,
		projectID:   projectID,
		repoID:      repoID,
		worktree:    worktree,
		tmpDir:      tmpDir,
	}, nil
}

// spawnContext is the input renderSpawnContext folds into one expansion context.
type spawnContext struct {
	chatID        string
	workspaceID   string
	providerID    string
	projectID     string
	repoID        string
	runnerID      string
	tmpDir        string
	worktree      string
	crowbarHome   string
	threads       string
	conversation  string
	promptMessage string
	gapTurns      int
	resuming      bool
	selection     engineagents.Selection
}

// renderSpawnContext builds the expansion context every descriptor step is
// rendered against, and answers WHETHER Crowbar's context document is delivered
// on this spawn at all.
func (u *Usecase) renderSpawnContext(
	in spawnContext,
) (engineagents.TemplateCtx, bool) {
	tctx := engineagents.TemplateCtx{
		Tmp:         in.tmpDir,
		Cwd:         in.worktree,
		CrowbarHook: u.crowbarHookPath(in.crowbarHome),
		Segid:       in.runnerID,
		// The credential the descriptors hand `crowbar mcp` so this runner's tool
		// calls can be attributed to it. Minted here because this is where the
		// runner id is born, and by the SAME minter DispatchMCP verifies against —
		// a token minted anywhere else would authenticate nothing.
		RunnerToken: u.mintRunnerToken(in.runnerID),
		Provider:    in.providerID,
		ProjectID:   in.projectID,
		RepoID:      in.repoID,
		WorkspaceID: in.workspaceID,
		ChatID:      in.chatID,
		GapTurns:    strconv.Itoa(in.gapTurns),
		Message:     in.promptMessage,
		// Read by the descriptor's own model:/effort: apply steps and by nothing
		// else. They are empty on a chat that has chosen nothing, and the steps
		// that would read them are not rendered at all in that case.
		Model:  in.selection.Model,
		Effort: in.selection.Effort,
	}
	// The message for a provider that can ONLY be reached through a user message (a
	// resumed codex ignores every config channel). It POINTS at the ledger already on
	// disk and says where to start reading — it never carries the transcript, which
	// would dump the whole handed-off exchange into the chat for the user to scroll
	// past. An agent reads files.
	tctx.ContextPointer = engineagents.Expand(config.GetPrompts().HandoffPointer, tctx)
	// The preamble first (which tools exist), then the lineage (which OTHER chats
	// this one reads), then the handoff (what was said here already). Orientation
	// before history: a model should know it has get_chat_log and which ids to
	// point it at before it reads a conversation it is told not to act on.
	tctx.Context = composeContext(
		engineagents.Expand(config.GetPrompts().CapabilitiesInstruction, tctx),
		in.threads,
		in.conversation,
	)
	// WHETHER to deliver that document at all, which is a separate question from what
	// it says.
	//
	// A handoff is something that HAPPENED and is always worth delivering. The
	// capability preamble is standing orientation, so it may only drive delivery down a
	// SILENT channel: a fresh spawn's ContextInject is a config key or a flag, but a
	// resume's ResumeContextInject can be a USER MESSAGE — a resumed codex can be
	// reached no other way (see codex.yaml) — and reopening a closed tab resumes a
	// provider with nothing recorded in between. Letting the preamble deliver there
	// would open every revived codex chat with a "while you were away" pointer about
	// nothing, and codex answers its opening message on sight.
	//
	// The cost is that a provider whose resume channel IS silent (claude's is a flag)
	// also loses the preamble on a gapless revive. That is deliberate over the
	// alternative: whether a resume channel speaks out loud is the DESCRIPTOR's
	// knowledge, and inventing a "this channel is silent" field to recover one
	// directive would put a provider's manners in this package. Its tools are still
	// registered on that spawn either way — only the directive is missing.
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}

// spawnPreflight is everything a spawn must know BEFORE it touches the
// filesystem or forks anything: whether the provider may run at all, whether its
// tool surface is switched on, what this chat reads besides its own
// conversation, and what it asked to run as.
//
// All four are resolved together and ahead of every side effect, so a preference
// the daemon cannot read, a lineage it cannot resolve or a selection it cannot
// fold fails the spawn without leaving a tmp dir or a process to unwind.
type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
}

func (u *Usecase) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	create bool,
) (spawnPreflight, error) {
	// This is the ONE seam every vendor CLI is launched through, which makes it the
	// only place a disabled provider can actually be stopped.
	if err := u.requireProviderEnabled(ctx, providerID); err != nil {
		return spawnPreflight{}, err
	}
	// The tool switch is a SEPARATE axis from whether the provider is enabled: a CLI
	// spawned with its tools off still comes up, still fires its hooks and still
	// holds a normal chat.
	mcpOn, err := u.providerMCPEnabled(ctx, providerID)
	if err != nil {
		return spawnPreflight{}, err
	}
	threads, err := u.threadContext(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	// The selection is also what gets RECORDED on the runner, so the record and the
	// argv are rendered from ONE read: two reads could disagree across a concurrent
	// change and leave a process whose recorded selection is not the one it runs.
	sel, err := u.chatSelection(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	return spawnPreflight{mcpOn: mcpOn, threads: threads, selection: sel}, nil
}

// forkRequest is everything the fork itself needs, gathered so the barrier, the
// fork and their shared unwind live in one place.
type forkRequest struct {
	runnerID    string
	providerID  string
	workspaceID string
	worktree    string
	crowbarHome string
	tmpDir      string
	argv        []string
	env         []string
}

// forkCLI installs the hook startup barrier and forks the vendor CLI, unwinding
// both on failure.
//
// The barrier goes in IMMEDIATELY before the fork and not a line earlier: a
// provider can synchronously fire SessionStart and UserPromptSubmit from inside
// CreateCommand, before the terminal session id exists and therefore before the
// runner can be persisted. No process exists before this point, and every hook
// after it is either buffered or observes the durable row once the barrier lifts.
//
// The unwind is shared because both failures leave the same debris: an
// injected-context entry that only onExit would ever forget, and a tmp dir that
// only onExit would ever remove — and onExit never runs for a CLI that never
// lived. RemoveUnderHome is guarded by crowbarHome, so a poisoned chats dir can
// never make that rm escape the user's real filesystem.
func (u *Usecase) forkCLI(
	ctx context.Context,
	req forkRequest,
) (string, error) {
	if err := u.pendingHooks.register(req.runnerID); err != nil {
		u.agents.ForgetRunner(req.runnerID)
		RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)
		return "", fmt.Errorf("agent: spawn runner: install hook startup barrier: %w", err)
	}
	termSessID, err := u.term.CreateCommand(ctx, req.workspaceID, req.worktree, req.argv, req.env,
		u.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir))
	if err == nil {
		return termSessID, nil
	}
	u.pendingHooks.discard(req.runnerID)
	u.agents.ForgetRunner(req.runnerID)
	RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)

	// A CLI that is not installed is the ONE spawn failure the user can act on, so
	// it travels as its own sentinel (→ 424, a named message in the UI) rather than
	// being buried in a wrap chain that maps to an opaque 500. The provider id, not
	// the resolved argv[0], is what the UI can name.
	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return "", fmt.Errorf("%w: %s", engineterminal.ErrCommandNotFound, req.providerID)
	}
	return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
}

// recordRunner persists what the spawn just created: the chat, if this is a fresh one, and
// the runner now placed on it. The CLI is ALREADY LIVE by the time we get here — a pure
// command cannot fork a process — so any failure tears it down rather than leaking a CLI
// nothing in Crowbar points at.
func (u *Usecase) recordRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	runnerID string,
	termSessID string,
	launchSessionID string,
	sel engineagents.Selection,
	create bool,
) error {
	now := time.Now()
	if create {
		created, err := u.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: workspaceID,
			Now:         now,
		})
		if err != nil {
			return u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
		u.work.set(chatID, created.Working)
	}
	if _, err := u.runners.Start(ctx, agentrunner.StartInput{
		RunnerID:        runnerID,
		WorkspaceID:     workspaceID,
		ProviderID:      providerID,
		TerminalSession: termSessID,
		ChatID:          chatID,
		LaunchSessionID: launchSessionID,
		// The selection this process was ACTUALLY launched with, recorded from the
		// same read that rendered its argv. It is the only authority on what this
		// CLI is running: nothing can ask the process later.
		LaunchModel:  sel.Model,
		LaunchEffort: sel.Effort,
		Now:          now,
	}); err != nil {
		return u.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
			fmt.Errorf("agent: spawn runner: start runner: %w", err))
	}

	// A Start is a PLACEMENT, so it obeys the same rule a Move does: whoever else is on this
	// chat is retired. The spawn gate cannot cover this, and it is not a hairline window —
	// it is as wide as a process fork:
	//
	//	a gated SwitchProvider quits and DISPLACES the outgoing CLI, leaving the chat with
	//	nobody on it; a HOOK (never gated, and never may be) moves another live CLI onto it,
	//	evicting nobody because nobody is there; and only THEN do we resolve a descriptor,
	//	render a tmp dir, fork a process and land here.
	//
	// Without this the chat ends up holding both, indefinitely — and the loser is INVISIBLE,
	// because the serving read hands out the newest arrival while the other one goes on
	// appending to the chat's ledger. Start is SendWait, so this read already sees us.
	u.retireOthersOn(ctx, chatID, runnerID)
	return nil
}

// retireOthersOn enforces I2 at a placement: after a runner is placed on a chat, every OLDER
// runner still on that chat is taken off it and killed. It is called by BOTH of the
// placement paths that can land on a chat somebody else may already be on — Start (a spawn)
// and Move (a /resume that lands on a known chat).
//
// (There is a third Move, moveToNewChat, and it deliberately does not call this: its
// destination is a uuid minted two lines earlier, so nothing can possibly be placed there.
// Calling it would add a read to the hot /clear path to discover a fact we already know.)
func (u *Usecase) retireOthersOn(
	ctx context.Context,
	chatID string,
	keepID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners placed on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	u.retireOlderThan(ctx, placed, keepID, "chat", chatID)
}

// evictHolderOf enforces I3 at a placement: after a runner takes a conversation (a bind or a
// move), every OLDER runner still holding it is evicted. Two CLIs on one provider session id
// both write the same session file and corrupt it — that constraint is the PROVIDER's, not
// ours, which is why the incumbent is displaced rather than the newcomer refused.
func (u *Usecase) evictHolderOf(
	ctx context.Context,
	runner domain.AgentRunner,
	sessionID string,
) {
	holders, err := u.runners.LiveRunnersForSession(ctx, runner.WorkspaceID, sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners holding this conversation (best-effort, continuing)",
			"session_id", sessionID, "err", err)
		return
	}
	u.retireOlderThan(ctx, holders, runner.ID, "conversation", sessionID)
}

// retireOlderThan is the whole rule, and both invariants are the same rule: OF EVERYONE HERE,
// THE NEWEST ARRIVAL STAYS AND THE REST GO. `here` is a chat (I2) or a conversation (I3);
// `everyone` is the plural read, newest arrival first; `me` is keepID.
//
// It reads ALL of them, not one. A single-row read cannot serve a placement at all: once the
// write has committed the caller IS the newest, so it would be handed back its own row and
// evict nobody — and if the invariant were somehow already broken, evicting one of three
// would heal nothing.
//
// ASYMMETRY IS LOAD-BEARING — retire only the arrivals strictly OLDER than me, never the
// newer ones, and NOBODY AT ALL if I am not on the list. "Retire everyone else" reads more
// natural and is wrong: two placements landing on one chat within microseconds each see both
// rows, and each kills the other. Both CLIs die, the chat is left with NOBODY, and
// SwitchProvider cheerfully returns the id of a runner it has just SIGTERM'd — so the pane
// attaches to a dying PTY. With the asymmetry exactly one of them (the newest) retires
// anybody, and it is the same runner the serving reads hand out, because they order by the
// same key. The retire rule and the read model agree by construction rather than by luck.
//
// keepID absent means somebody else's placement has already taken me off this chat: I am no
// longer here, so it is not mine to police.
//
// Best-effort throughout: the placement it follows is already recorded, and the CLIs it is
// chasing are ones Crowbar wants gone, not ones the user is waiting on.
func (u *Usecase) retireOlderThan(
	ctx context.Context,
	present []domain.AgentRunner,
	keepID string,
	whereKind string,
	whereID string,
) {
	me := -1
	for i, r := range present {
		if r.ID == keepID {
			me = i
			break
		}
	}
	if me < 0 {
		return
	}
	for _, older := range present[me+1:] {
		slog.WarnContext(ctx, "agent: another CLI was already here; retiring it",
			whereKind, whereID, "keeping", keepID, "retiring", older.ID)
		u.retire(ctx, older)
	}
}

// teardownAfterPersistFailure kills a CLI that is already live but whose runner
// could not be recorded, so the failure leaves no orphan process behind — a CLI
// nothing in Crowbar points at is invisible, and an invisible agent is the worst
// state this system can be in. The original error is returned wrapped, so an
// ErrValidation still classifies as a conflict upstream. TerminateGraceful's own
// "session already gone" is harmless here and ignored.
func (u *Usecase) teardownAfterPersistFailure(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := u.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown after persist failure",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	return cause
}

// RemoveUnderHome is the SINGLE guarded os.RemoveAll every agent-path filesystem
// removal routes through. It removes target ONLY when target is provably strictly
// under crowbar home — worktreepath.UnderHome, the exact strict-prefix check the
// worktree removers re-assert on their own root. AgentChatsDir already reroots chat
// state under home, but filepath.Join CLEANS "..", so a crafted repo remote slug
// (host/owner/../../..) could in principle collapse a derived path OUTSIDE home;
// this re-asserts the invariant AT the rm so no agent removal can EVER reach the
// user's real filesystem, even if a caller is handed a poisoned chats dir. A target
// not under home (including a blank/unresolvable home) is logged and skipped, never
// removed — fail-closed. Callers are all best-effort, so a plain rm error is logged,
// not returned.
//
// Exported so the workspace-delete cascade's on-disk reap seam (app.reapAgentChatFiles,
// wired into repositories.Container.ReapChatFiles) can route through the exact SAME
// guard PurgeChat uses, rather than a package outside agent reimplementing the check.
func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	if !worktreepath.UnderHome(target, home) {
		slog.WarnContext(ctx, "agent: refusing to rm agent path outside crowbar home (skipping)",
			"target", target, "home", home)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		slog.WarnContext(ctx, "agent: reap agent path", "target", target, "err", err)
	}
}

// onRunnerExit builds the CreateCommand onExit callback for runnerID: the PTY has
// died, which is the ONE fact that makes a runner dead. home is captured at spawn
// time (the same home tmpDir was created under) so the guard needs no post-hoc
// lookup that a since-deleted workspace could fail. It runs on a background context:
// the terminal engine invokes onExit from its own reap goroutine, well after any
// request context that spawned this runner could have been cancelled.
func (u *Usecase) onRunnerExit(home, runnerID, tmpDir string) func() {
	return func() {
		RemoveUnderHome(context.Background(), home, tmpDir)
		// CreateCommand can observe process exit before recordRunner has a
		// terminal-session id to persist. The startup barrier remembers that fact;
		// spawnRunner reconciles it immediately after persistence and ordered hook
		// replay, instead of this callback missing the not-yet-existing row forever.
		if u.pendingHooks.markExited(runnerID) {
			return
		}
		u.reconcileRunnerExit(context.Background(), runnerID)
	}
}

// reconcileRunnerExit records the death of a CLI. It is the ONLY place a runner is
// Exited at runtime, and it runs because the PROCESS died — never because Crowbar
// formed an opinion that it should be dead. Every teardown path in this package
// (provider switch, eviction, chat delete, workspace delete) converges here through
// the same door: they all just SIGTERM the PTY.
//
// It also closes a turn the dead CLI left open. A CLI that dies mid-turn will never
// send its turn_stop hook, so without this the chat stays Working forever — a
// spinner that never stops, on a chat with nothing running.
//
// That turn is closed ONLY if the chat has no OTHER runner on it now. A provider
// switch starts the incoming CLI while the outgoing one is still dying, so a belated
// exit must never close the INCOMING runner's turn — and once the runner's own row is
// gone (Exit, above), "is anyone still on this chat" is exactly the query
// LiveRunnerForChat. The old code answered the same question with "am I still the
// chat's ACTIVE SEGMENT", which is the stored-liveness flag this refactor deletes.
//
// Errors are logged, not returned: onExit runs off the terminal engine's reap
// goroutine and there is no caller to hand an error to.
func (u *Usecase) reconcileRunnerExit(ctx context.Context, runnerID string) {
	// The echo guard is per-spawn and means nothing once the process is gone.
	u.agents.ForgetRunner(runnerID)
	// Nor does a prompt it was blocked on. Its hooks may still be ALIVE — they are
	// spawned detached, so killing a CLI orphans them — so every relay this runner
	// owned is woken with no verdict, and every question it was asking is closed.
	// A pending prompt over a process that no longer exists is a banner nothing
	// else will ever clear.
	u.releaseAnswerWaiters(ctx, runnerID)
	// Nor does an in-flight turn: a CLI that died mid-answer will never send the turn_stop
	// that closes it, so a provider switch parked on that turn is released by the DEATH
	// instead — the same real signal that ends the runner, and the reason the wait needs no
	// timeout to be safe against a CLI that falls over.
	u.turns.complete(runnerID)

	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if !errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: reconcile runner exit: get runner", "runner_id", runnerID, "err", err)
		}
		// Already exited (a double exit is not an error — the row is simply gone).
		return
	}
	u.reconcilePromptRunnerDeparture(ctx, runner, runner.CurrentChatID)
	if _, err := u.runners.Exit(ctx, runnerID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: reconcile runner exit: exit runner", "runner_id", runnerID, "err", err)
		return
	}

	// Close a turn it left open — unless it had already been DISPLACED, in which case its
	// chat (if it still had a turn to close) was dealt with at displacement time and
	// CurrentChatID is now empty, meaning nowhere.
	u.closeAbandonedTurn(ctx, runner.CurrentChatID)
}

func (u *Usecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}

// IngestHook maps an incoming vendor hook to a canonical event and applies it: a
// session_start goes to the context-move reducer, a user_prompt opens the turn and
// appends it to the ledger, a turn_stop closes the turn and appends the reply.
//
// EVERY hook is routed by resolving runnerID → runner → CurrentChatID from DURABLE
// state. Nothing in memory can disagree with it, because there is no longer anything
// in memory to disagree: this is what stops an orphaned CLI's turns landing in a chat
// it has left (spec §4.7). A hook from a runner we do not know, or for a chat that no
// longer exists, or with a malformed payload, is ignored — never an error. A hook must
// never break the vendor CLI's turn.
func (u *Usecase) IngestHook(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	// Check the startup barrier BEFORE the repository. Once recordRunner commits,
	// the row is visible, but the barrier deliberately remains installed through
	// ordered replay; consulting only the repository here would let a later hook
	// overtake the buffered session_start/user_prompt batch.
	if handled, err := u.pendingHooks.enqueue(runnerID, provider, canonicalEvent, rawPayload); handled {
		return err
	}
	return u.ingestHookNow(ctx, runnerID, provider, canonicalEvent, rawPayload)
}

// ingestHookNow is the persisted-runner path. Startup replay calls it directly
// so replayed hooks do not enqueue themselves back into their own barrier.
func (u *Usecase) ingestHookNow(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	if canonicalEvent == "user_prompt" {
		return u.ingestUserPromptInterlocked(ctx, runnerID, provider, canonicalEvent, rawPayload)
	}
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if errors.Is(err, agentrunner.ErrNotFound) {
			// A hook from a runner we do not know (or one whose PTY has already died):
			// ignore it. Never resurrect a runner from a hook.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: runner: %w", err)
	}
	return u.ingestResolvedHook(ctx, runner, provider, canonicalEvent, rawPayload)
}

// ingestUserPromptInterlocked makes "start a turn" atomic with Crowbar's final
// idle-check-and-displace section. It re-reads placement after acquiring the
// chat lock: a hook that resolved the outgoing runner just before a replacement
// displaced it must not start a turn on that chat using its stale struct.
func (u *Usecase) ingestUserPromptInterlocked(
	ctx context.Context,
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	for {
		runner, err := u.runners.Get(ctx, runnerID)
		if err != nil {
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: runner: %w", err)
		}
		if runner.CurrentChatID == "" {
			return nil
		}
		chatID := runner.CurrentChatID
		unlock := u.turnStarts.lock(chatID)
		current, err := u.runners.Get(ctx, runnerID)
		if err != nil {
			unlock()
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: refresh runner under turn-start interlock: %w", err)
		}
		if current.CurrentChatID != chatID {
			unlock()
			// Placement changed before the lock. Retry against the durable current
			// chat, or drop on the next iteration if the runner was displaced.
			continue
		}
		err = u.ingestResolvedHook(ctx, current, provider, canonicalEvent, rawPayload)
		unlock()
		return err
	}
}

func (u *Usecase) ingestResolvedHook(
	ctx context.Context,
	runner domain.AgentRunner,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	runnerID := runner.ID

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The runner is the source of truth for which provider spawned this CLI. The
	// hook's self-reported provider is only a guard against a mis-authored descriptor.
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}

	descriptor, err := u.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	// Telemetry arrives over the same relay as a hook — a command, JSON on stdin,
	// scoped to the same segment — but it is not a conversation event. It carries
	// no session, describes no turn, and must not be run through the ownership
	// guard, which asks a question about conversations.
	if canonicalEvent == engineagents.HookTelemetry {
		return u.handleTelemetry(ctx, runner, descriptor, rawPayload)
	}

	// One call, deliberately: decode, ownership-check and map are fused in the
	// engine so the middle step cannot be skipped. It is the guard that stops a
	// provider's own internal session being filed as the user's conversation, and
	// a guard a caller can forget to call is one that will eventually be forgotten.
	//
	// Every failure here DROPS the hook rather than failing it. A hook must never
	// break the vendor CLI's turn.
	ev, err := descriptor.ParseHook(canonicalEvent, rawPayload)
	if err != nil {
		switch {
		case errors.Is(err, engineagents.ErrForeignConversation):
			slog.DebugContext(ctx, "agent: ingest hook: dropping a hook that is not this CLI's own conversation",
				"reason", err, "event", canonicalEvent,
				"provider", runner.ProviderID, "runner_id", runnerID)
		case errors.Is(err, engineagents.ErrHookUndeclared):
			slog.DebugContext(ctx, "agent: ingest hook: provider does not map this event",
				"event", canonicalEvent, "provider", runner.ProviderID, "runner_id", runnerID)
		default:
			slog.WarnContext(ctx, "agent: ingest hook: parse payload",
				"err", err, "event", canonicalEvent, "runner_id", runnerID)
		}
		return nil
	}

	// Where does this conversation's own transcript stand RIGHT NOW? Asked on every
	// hook and answered once per file: a session Crowbar has not been watching must
	// start at the file's end, or a resumed conversation's whole history would be
	// replayed into the record as though the agent had just said all of it. Asked
	// here rather than on the announcement alone because a daemon that restarts
	// mid-session never sees that session's announcement.

	switch ev.Kind {
	case engineagents.HookSessionStart:
		return u.handleSessionStart(ctx, runner, ev)
	case engineagents.HookUserPrompt, engineagents.HookTurnStop, engineagents.HookTurnFailed:
		return u.handleTurn(ctx, runner, descriptor, ev)
	case engineagents.HookMessageDelta,
		engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail,
		engineagents.HookSubagentPre, engineagents.HookSubagentPost,
		engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation,
		engineagents.HookCompactPre, engineagents.HookCompactPost:
		return u.handleObservation(ctx, runner, descriptor, ev, rawPayload)
	case engineagents.HookSessionEnd:
		// A session ending is already observed authoritatively by the PTY exit
		// reconcile, which runs whether or not the CLI got to fire a hook. Acting
		// on it here as well would close a turn twice.
		return nil
	}
	return nil
}

// chatForRunner resolves the chat a hook belongs to: the one the runner is on RIGHT
// NOW, read from the runner, which the preceding session_start has already moved if
// the CLI changed conversation. ok=false means the hook belongs nowhere and must be
// dropped — never an error, because a hook must never break the vendor CLI's turn.
func (u *Usecase) chatForRunner(
	ctx context.Context,
	runner domain.AgentRunner,
) (domain.AgentChat, bool, error) {
	if runner.CurrentChatID == "" {
		// The runner is placed NOWHERE: Crowbar has taken it off its chat and is killing
		// it (a switch, an eviction, a chat deleted under it), and a SIGTERM'd CLI keeps
		// talking for a moment. Its turns belong to nobody, and nowhere is never looked up
		// — GetChat("") would miss and trigger agentchat's lazy self-heal, replaying the
		// ENTIRE event log, on every hook of a dying CLI.
		return domain.AgentChat{}, false, nil
	}
	chat, err := u.chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			// The chat was deleted out from under the CLI (which is still dying). A turn
			// typed into a chat the user has just removed goes nowhere, by design.
			return domain.AgentChat{}, false, nil
		}
		return domain.AgentChat{}, false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	return chat, true, nil
}

// openAssistantTurn starts the turn that this prompt's activity attaches to.
//
// Best-effort: the conversation record is what makes a chat legible, but failing
// the hook would break the vendor CLI's turn, and the close path recovers anyway
// by recording the reply on its own terms.
func (u *Usecase) openAssistantTurn(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
) {
	if err := u.activity.OpenTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     openTurnID(chat.ID, runner.ID),
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  runner.CurrentSession,
		Now:        time.Now(),
	}); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: open assistant turn",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
}

// openTurnID is the PLACEHOLDER identity a reply carries while it is being
// produced, so tool calls have something to attach to before any reply exists.
//
// It is derived rather than random because the open and the close are separate
// hooks and neither carries state between them; a runner produces one reply at a
// time, which is what makes chat+runner unique for as long as the placeholder
// lives. It never survives the turn: CloseTurn re-keys the reply onto the hook's
// delivery id and the projection re-points the activity with it.
func openTurnID(chatID, runnerID string) string {
	return "open-" + chatID + "-" + runnerID
}

// handleTurn applies a turn hook to the chat the runner is on RIGHT NOW — read from
// the runner, which the preceding session_start has already moved if the CLI changed
// conversation (a provider announces the switch BEFORE the turn that follows it, so
// no turn is ever misfiled).
//
// It takes the resolved agent for one question only: WHOSE words a user_prompt is
// carrying. That is answered from the provider's own declarations (see
// engineagents.MatchInjectedPrompt), which is the same reason handleObservation is
// handed one.
func (u *Usecase) handleTurn(
	ctx context.Context,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	chat, ok, err := u.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}

	switch ev.Kind {
	case "user_prompt":
		return u.openTurnFromPrompt(ctx, chat, runner, agent, ev)
	case "turn_stop":
		return u.closeTurnFromStop(ctx, chat, runner, agent, ev)
	case engineagents.HookTurnFailed:
		return u.closeTurnFromFailure(ctx, chat, runner, ev)
	}
	return nil
}

// openTurnFromPrompt applies a user-prompt hook: it decides whose words arrived,
// records them under that author, and opens the reply they are about to produce.
func (u *Usecase) openTurnFromPrompt(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	// WHOSE WORDS ARE THESE? Three different authors reach this one hook — the
	// user, Crowbar's own injected handoff, and the provider's harness — and the
	// two checks below are what tell them apart. Neither is a guess about the
	// runner: a prompt Crowbar delivered arrived in the argv of the process
	// Crowbar spawned, so the only thing left to classify here is content.
	//
	// Crowbar's own context document coming back at us: a provider whose only
	// resume channel is a user message (codex) fires user_prompt with the very
	// handoff we injected. That is not something the user said — recording it would
	// put the handoff in the ledger as a "user" turn, and the NEXT handoff would
	// then quote it inside itself (the nesting seen live). Drop it from the ledger
	// and from title derivation, but still open the turn: the CLI really is working
	// on it, and the workspace's working overlay must say so.
	if u.agents.WasInjected(runner.ID, ev.Message) {
		started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		u.work.set(chat.ID, started.Working)
		u.turns.begin(runner.ID, chat.ID)
		u.openAssistantTurn(ctx, chat, runner)
		return nil
	}
	// The PROVIDER's own harness talking to its own model on the user's hook: a
	// background-subagent completion report is the measured case, and the ledger
	// recorded every one of them as something the user said. It is the sibling of
	// the branch above and deliberately not a copy of it — that one drops the text
	// because Crowbar wrote it and already has it, and this one must NOT, because
	// this text is real context the agent received and its next answer refers to
	// it. Dropped, the reply would have no antecedent; attributed, the user is
	// quoted saying something they never wrote, which is what get_chat_log was
	// serving to other agents. So it is recorded under its own role.
	//
	// No derived title: a chat named after a subagent's completion report is named
	// after nothing its user did. The turn still opens — the agent genuinely is
	// about to work on this — and no prompt-delivery journal is advanced, because
	// nothing Crowbar queued was accepted here.
	if injected, ok := engineagents.MatchInjectedPrompt(agent, ev.Message); ok {
		slog.DebugContext(ctx, "agent: ingest hook: user_prompt was injected by the provider's harness",
			"chat_id", chat.ID, "runner_id", runner.ID, "provider", runner.ProviderID,
			"kind", injected.Kind, "needle", injected.Needle)
		started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		u.work.set(chat.ID, started.Working)
		u.turns.begin(runner.ID, chat.ID)
		appendErr := u.appendRunnerTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleHarness, ev.Message,
		)
		u.openAssistantTurn(ctx, chat, runner)
		return appendErr
	}
	if err := u.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
	}
	// A user prompt opens the turn: mark the chat Working so the read model (and
	// the workspace spinner) see a live turn.
	started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
	if err != nil {
		return fmt.Errorf("agent: ingest hook: start turn: %w", err)
	}
	u.work.set(chat.ID, started.Working)
	// And record it as IN FLIGHT, which is the same fact without the read model's lag
	// in front of it — a provider switch blocks on this rather than on Working, so that
	// it never quits a CLI that is still answering (turnWaits).
	u.turns.begin(runner.ID, chat.ID)
	appendErr := u.appendRunnerTurn(
		ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
		domain.TurnRoleUser, ev.Message,
	)
	// The reply this prompt is about to produce, opened NOW so the tool calls,
	// subagents and interruptions that follow attach to it. Without an open turn
	// each of them would open one of its own, and the reply recorded at turn_stop
	// would be a separate record — leaving the UI unable to say which activity
	// produced which answer.
	u.openAssistantTurn(ctx, chat, runner)
	// The hook is the provider's acknowledgement that the argv prompt was
	// accepted. Advance the journal even when the ledger write failed: the hook
	// itself is positive delivery evidence, and leaving the request spawned
	// would wedge every future prompt. Conversely, a journal failure after a
	// successful ledger append is repaired from that attributed turn by the
	// turn_stop and pre-destructive reconciliation paths.
	confirmErr := u.confirmPromptAccepted(ctx, chat, runner, ev.Message)
	if appendErr != nil {
		return appendErr
	}
	if confirmErr != nil {
		return fmt.Errorf("agent: confirm React prompt acceptance: %w", confirmErr)
	}
	return nil
}

// closeTurnFromStop applies a turn-stop hook: the answer lands in the ledger
// BEFORE anybody is told the turn ended, and the turn's waiters are released only
// once it is there.
func (u *Usecase) closeTurnFromStop(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	// THE ANSWER IS DURABLE BEFORE ANYBODY IS TOLD THE TURN ENDED. StopTurn's
	// projection broadcasts Working=false, and the React chat treats that edge as
	// its cue to do ONE ledger read and then stop polling (spec §6). Publishing the
	// state change first raced this append: the read could be served before the
	// assistant row existed, and with the turn over and the queue empty nothing ever
	// re-read it — the reply sat in the ledger, invisible, until an unrelated
	// refresh (a chat switch, a reload) happened to fire. Observed live 2026-08-16.
	//
	// Ordering this way costs nothing the old comment worried about: an empty
	// message is a ledger no-op here and StopTurn below still runs, and a FAILED
	// append still falls through to StopTurn rather than returning early, so the
	// turn state is never left open on a write error.
	appendErr := u.closeAssistantTurn(ctx, chat, runner, ev)
	// Released only ONCE THE LEDGER HAS THE ANSWER: a switch waiting on this turn
	// reads the ledger the moment it wakes, to assemble the handoff. Waking it
	// earlier would hand the incoming CLI a conversation missing the very turn the
	// switch waited for. Deferred so a failed StopTurn still releases the waiter —
	// the turn is over either way, and a switch parked on it would never wake.
	defer u.turns.complete(runner.ID)
	// The turn ended — which is NOT the same fact as the agent being done, so this
	// carries the CLI's own count of what it left running (ev.AsyncWork) and lets the
	// aggregate fold Working from both. A CLI that hands work to a background task
	// ends its turn right here and goes quiet until that work reports back; clearing
	// Working on the strength of this hook alone is what darkened the spinner under a
	// live subagent. A provider that reports no such level sends 0 and gets exactly
	// the turn-only behaviour it had before.
	stopped, err := u.chats.StopTurn(ctx, chat.ID, time.Now(), ev.AsyncWork)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop turn: %w", err)
	}
	u.work.set(chat.ID, stopped.Working)
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.WarnContext(ctx, "agent: reconcile React prompt acceptance on turn stop",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
	return appendErr
}

// handleSessionStart reconciles a conversation announcement. By the time we are
// called the CLI has ALREADY switched — we RECORD what happened, and we never fail
// on anybody else's state (spec §3).
func (u *Usecase) handleSessionStart(
	ctx context.Context,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	if ev.SessionID == "" {
		return nil
	}

	// "Is this conversation one we know?" is answered from APPEND-ONLY history, so it
	// keeps answering long after the runner that opened the conversation has died —
	// which is what makes a /resume into a dormant chat recognisable instead of
	// looking brand new. It replaces the in-memory sessionToChat map AND the boot
	// reseed that used to repopulate it.
	knownChatID, err := u.runners.ChatForSession(ctx, runner.WorkspaceID, ev.SessionID)
	known := err == nil
	if err != nil && !errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("agent: ingest hook: lookup session: %w", err)
	}

	switch d := engineagents.Decide(runner.CurrentSession, ev.SessionID, knownChatID, known); d.Kind {
	case engineagents.MoveNoop:
		return nil
	case engineagents.MoveBind:
		// The conversation is recorded AGAINST THE CHAT the runner is on, so that chat had
		// better still be there. It might not be: PurgeChat kills the CLI but a SIGTERM is
		// not synchronous, so a chat deleted seconds after it was created ("wrong provider,
		// undo") can be gone before its CLI has even announced. Binding anyway would write a
		// conversation-history row against a hard-deleted chat — a dangling /resume target,
		// the exact live trap PurgeChat drops that history to prevent.
		ok, err := u.requirePlacement(ctx, runner, runner.CurrentChatID)
		if err != nil || !ok {
			return err
		}
		if _, err := u.runners.BindSession(ctx, runner.ID, ev.SessionID, known, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: bind session: %w", err)
		}
		// A bind takes a conversation, so it obeys I3 exactly as a move does: whoever else is
		// live on that conversation is evicted. On every legitimate path this is a NO-OP, and
		// it is worth knowing why — a CLI's first conversation is normally one Crowbar itself
		// chose, either brand new (an id nobody can be holding) or a resume taken under the
		// chat's spawn gate, having already quit whoever was there.
		//
		// It is a guard rather than a comment because that argument is breakable FROM A
		// CONFIG FILE. ResolveDescriptor merges on-disk descriptor overrides out of crowbar
		// home, and spawn.args is the user's to edit: an override adding `--continue`, or any
		// provider that auto-restores its last session, makes a freshly spawned CLI announce
		// an id CROWBAR NEVER CHOSE — possibly one another live runner is holding. Without
		// this, two CLIs would then write one provider session file and corrupt the
		// PROVIDER'S OWN DATA, with no Go error and no log line. Provider knowledge lives in
		// YAML precisely so that Go never has to trust it; an invariant that depends on what
		// the YAML says is not an invariant.
		u.evictHolderOf(ctx, runner, ev.SessionID)
		return nil
	case engineagents.MoveToNew:
		return u.moveToNewChat(ctx, runner, ev.SessionID)
	case engineagents.MoveToKnown:
		// The destination is resolved from append-only history, which can outlive the chat
		// it names: PurgeChat's history drop is best-effort, and deleting a Crowbar chat
		// deliberately does NOT delete the vendor's session file — so a purged chat's
		// conversation can still be sitting in the CLI's own /resume picker. Pick it, and
		// an unguarded move would repoint the runner at a chat that does not exist: an
		// invisible CLI, writing nowhere, forever.
		ok, err := u.requirePlacement(ctx, runner, d.ChatID)
		if err != nil || !ok {
			return err
		}
		return u.moveToKnownChat(ctx, runner, d.ChatID, ev.SessionID)
	}
	return nil
}

// requirePlacement reports whether chatID still exists — i.e. whether it is somewhere a
// runner can legitimately be placed and written to.
//
// When it does not, the runner has nowhere to write and must not be left running: it is
// retired (taken off whatever it was pointed at, and asked to quit). The hook itself is
// DROPPED, never failed — a hook must never break the vendor CLI's turn, and there is
// nothing the CLI could do with an error about a chat the user deleted anyway.
//
// This is not the reducer refusing reality (spec §3): the CLI's conversation switch is
// still a fait accompli. It is Crowbar declining to invent a placement — recording a
// runner onto a chat that does not exist is not a truth about the world, it is a lie the
// read model would then have to live with.
func (u *Usecase) requirePlacement(
	ctx context.Context,
	runner domain.AgentRunner,
	chatID string,
) (bool, error) {
	if chatID == "" {
		// Already displaced (we are killing it, and it has not fallen over yet).
		u.retire(ctx, runner)
		return false, nil
	}
	_, err := u.chats.GetChat(ctx, chatID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, agentchat.ErrNotFound) {
		return false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	slog.WarnContext(ctx, "agent: ingest hook: the runner's chat no longer exists; dropping the announcement and retiring the CLI",
		"runner_id", runner.ID, "chat_id", chatID)
	u.retire(ctx, runner)
	return false, nil
}

// moveToNewChat handles /clear and /new: a conversation nobody has ever seen appeared
// under a live runner, so a chat is minted for it and the runner moves in.
//
// The create runs FIRST because a create cannot destroy anything: if the Move then
// fails, the worst outcome is a stray empty chat in the sidebar — annoying,
// self-evident, deletable. The chat the runner is LEAVING is not written to at all, so
// no failure here can damage it (spec §4.2). This is the one flow that is still two
// writes, and the ordering is what bounds its failure.
func (u *Usecase) moveToNewChat(
	ctx context.Context,
	runner domain.AgentRunner,
	sessionID string,
) error {
	newChatID := uuid.NewString()
	created, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          newChatID,
		WorkspaceID: runner.WorkspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return fmt.Errorf("agent: ingest hook: mint chat: %w", err)
	}
	u.work.set(newChatID, created.Working)
	if _, err := u.runners.Move(ctx, runner.ID, newChatID, sessionID, false, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to new chat: %w", err)
	}
	// This is the third placement site, and the ONLY one that evicts nobody. That is not an
	// omission: the destination is a uuid minted three lines up, so no runner can be on it and
	// no runner can be holding a conversation nobody has ever announced before. Adding the
	// reads here would put two queries on the hot /clear path to discover a fact we already
	// know for certain. (The other two — Start and moveToKnownChat — land on chats and
	// conversations that CAN be occupied, and both retire whoever is there.)
	//
	// A turn the runner had open on the chat it just LEFT ends here, wherever it had got to:
	// its turn_stop will land on the NEW chat, so nothing will ever close it on the old one.
	// Releasing it is what stops a switch on the old chat waiting for an answer that is now
	// being written somewhere else.
	u.turns.complete(runner.ID)
	// And the turn must be closed on the chat itself, not just released in memory. Left open
	// it is durable: the chat reads Working forever, and the workspace's derived overlay
	// keeps it in the mid-turn set for the life of the daemon — a sidebar spinner running
	// over a workspace where nothing is happening. Same reasoning as the displace path, and
	// the same guards decide it (see closeAbandonedTurn): the runner has gone, and if a
	// successor has already taken the chat then the turn is not ours to close.
	u.closeAbandonedTurn(ctx, runner.CurrentChatID)
	return nil
}

// moveToKnownChat handles /resume into a conversation we know: the runner is repointed
// at the chat that owns it, and any runner already HOLDING that conversation is evicted.
//
// The eviction is forced by invariant I3 — at most one live runner per conversation,
// because two CLIs on one provider session id both write the same session file and
// corrupt it. That constraint is the PROVIDER's, not ours, and the CLI has already
// joined the conversation, so evicting the incumbent is the only move available.
//
// ORDER: Move FIRST (record reality — one write, one aggregate, and it cannot fail on
// anyone else's state), evict SECOND. If the kill then fails, our record is still
// ACCURATE — two runners really do hold the conversation — and only the cleanup needs a
// retry. The reverse order, ending something before recording reality, is literally the
// bug that bricked a chat.
func (u *Usecase) moveToKnownChat(
	ctx context.Context,
	runner domain.AgentRunner,
	toChatID string,
	sessionID string,
) error {
	if _, err := u.runners.Move(ctx, runner.ID, toChatID, sessionID, true, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to known chat: %w", err)
	}
	// Whatever it was mid-way through on the chat it just left is over there (see
	// moveToNewChat) — released in memory, and closed on the chat so the vacated chat
	// cannot go on advertising a turn whose turn_stop is landing elsewhere.
	u.turns.complete(runner.ID)
	if runner.CurrentChatID != toChatID {
		u.closeAbandonedTurn(ctx, runner.CurrentChatID)
	}

	// Whoever else is live on the CONVERSATION must go (invariant I3).
	u.evictHolderOf(ctx, runner, sessionID)

	// And whoever else is PLACED ON THE CHAT must go too (invariant I2) — a different
	// question from I3, and answering only I3 left the reported bug alive with one variable
	// changed:
	//
	//	Chat B is being worked by codex, which has not announced its conversation yet. B's
	//	older claude conversation is still in Crowbar's history AND still in claude's own
	//	/resume picker (Crowbar never deletes a vendor's session file). Resume it from another
	//	chat's CLI: nobody HOLDS that conversation, so nothing was evicted — and chat B ended
	//	up with two live CLIs on it, indefinitely, both writing its ledger, one invisible.
	//
	// Read AFTER the Move (which is SendWait, so we see ourselves) and retire everyone but
	// us: it is the same rule, and the same call, a Start makes. Both placement paths leave
	// exactly one runner on the chat, which is what makes I2 an invariant rather than a
	// coincidence.
	u.retireOthersOn(ctx, toChatID, runner.ID)
	return nil
}

// appendTurn records one conversation turn (user or assistant) into the chat's
// ledger. Empty text is a no-op. The turn's lifecycle frame is emitted by the
// StartTurn/StopTurn events the caller issues (fanned out by the hub projection),
// not from here — the ledger is a content log, not an aggregate.
//
// The turn is tagged with the provider of the RUNNER that produced it, which is what
// lets a later handoff say "assistant (codex): …" and lets the resume path ask when a
// given provider last spoke.
func (u *Usecase) appendTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	role, text string,
) error {
	return u.appendRunnerTurn(ctx, chat, providerID, "", "", role, text)
}

func (u *Usecase) appendRunnerTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID, runnerID, sessionID string,
	role, text string,
) error {
	if err := u.recordTurn(ctx, chat, providerID, runnerID, sessionID, role, text, ""); err != nil {
		return fmt.Errorf("agent: append turn: %w", err)
	}
	return nil
}

// NoteThreadLineage records, in chatID's OWN ledger, that a placement change has
// just given it a new chat ancestor: it is a thread of those chats FROM HERE ON.
//
// Re-parenting is not retroactive, and this is what makes that visible. Dragging a
// chat with fifty turns behind it under another chat does not mean those fifty turns
// were had with that context, and nothing rewrites them to pretend otherwise — a
// silent retroactive rewrite of what a chat has already read is the version nobody
// can audit afterwards. So the move is written down where everything else that
// happened to this chat is written down, in the append-only record its next spawn
// hands back to whatever CLI picks the chat up.
//
// It appends to the ledger and NOT to the aggregate. The lineage itself already
// lives on the aggregate as ParentID and needs no second copy; what is recorded here
// is the EVENT, at the position in the conversation where it happened, which is the
// only place the distinction between "read this all along" and "reads this from now
// on" can be expressed at all.
//
// A chat that has said NOTHING gets no note, which is the same rule read from the
// other end. The note dates the moment a chat began reading something it had not
// been reading; a chat with no turns has read nothing, so there is nothing above the
// line for "everything above this line" to refer to and nothing that a retroactive
// rewrite could have falsified. That covers the create path exactly: a chat born
// under a parent is placed before its CLI ever starts and is TOLD its lineage at
// spawn, so a note announcing a move it did not experience would be the only untrue
// line in its record.
func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: chat: %w", err)
	}
	turns, err := u.chatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return u.appendTurn(ctx, chat, lineageNoteProvider, "user", lineageNoteText(ancestors))
}

// lineageNoteProvider tags the ledger entries Crowbar writes ITSELF, as opposed to
// the ones it records from a vendor CLI's hooks. It can never collide with a real
// provider id, which matters more than it looks: Ledger.LastTurnAt(provider) decides
// whether a provider has a conversation on disk worth resuming, and a Crowbar note
// tagged with a provider's name would answer yes on that provider's behalf for a
// session it never held.
const lineageNoteProvider = "crowbar"

// lineageNoteText is the note itself, written in Go rather than in config.yaml's
// prompts like every other agent-facing text here. Those are prompts a user may
// blank or rewrite to taste; this one is a RECORD, and a record a user can silently
// empty is not one.
//
// It reads as a user-side turn because that is the side Crowbar speaks from when it
// addresses the agent (the handoff pointer is delivered as a user message for the
// same reason), and it opens with the [Crowbar] marker those messages carry so a
// model can tell the daemon apart from the person it is talking to.
func lineageNoteText(
	ancestors []string,
) string {
	return "[Crowbar] This chat was moved in the Chats panel and is a THREAD of " +
		strings.Join(ancestors, ", ") + " (nearest parent first) from this point on. " +
		"Read those chats with get_chat_log. Everything above this line was said BEFORE the move, " +
		"without any of that context: the move changes what this chat reads from now on and " +
		"rewrites nothing it has already read."
}

// AssembleHandoff resolves chatID's ledger directory, reads every entry, and wraps
// them in a legible preamble/footer so a freshly spawned provider CLI can be handed
// the prior context. Returns "" (not an error) when the ledger has no entries yet.
func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.assembleConversation(ctx, chatID, false, time.Time{})
}

// SwitchProvider is the headline provider handoff: it quits the chat's current
// vendor CLI, assembles a handoff from the ledger, and starts targetProviderID as a
// NEW runner on the SAME chat with that handoff injected. The chat does not move and
// is not written to; only the runner pointed at it changes.
//
// If targetProviderID left a conversation behind in this chat AND actually spoke in
// it, the new CLI is resumed into that very conversation (session.resume) and handed
// only the GAP — what happened under other providers while it was away. Otherwise it
// is spawned fresh with the whole ledger.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	defer u.spawns.lock(chatID)()
	return u.switchProviderLocked(ctx, chatID, targetProviderID)
}

// switchProviderLocked is SwitchProvider's body, with the chat's spawn gate ALREADY
// held. ResumeChat holds the same gate and calls straight in here — the gate is not
// re-entrant, and a revive that took it twice would deadlock on itself.
func (u *Usecase) switchProviderLocked(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	// REFUSE A DISABLED TARGET BEFORE ANYTHING IS TORN DOWN. spawnRunner guards it
	// too, but that guard fires at the END of this function — after the outgoing
	// CLI has already been quit — so a switch that only checked there would leave
	// the chat with no agent at all. ResumeChat enters here, so a dormant chat is
	// held to the same rule as a fresh one.
	if err := u.requireProviderEnabled(ctx, targetProviderID); err != nil {
		return "", err
	}
	for {
		chat, err := u.chats.GetChat(ctx, chatID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: chat: %w", err)
		}
		// Protect a React replacement that has not emitted its acceptance hook
		// yet. This first check happens before waiting; the interlocked check just
		// before displacement closes the hook-between-checks race.
		if err := u.requireNoPendingPromptDelivery(ctx, chat); err != nil {
			return "", err
		}
		// Resolve the target while the outgoing CLI is still alive. A missing or
		// malformed provider descriptor is a deterministic planning failure, not a
		// reason to destroy the user's current session and leave the chat dormant.
		crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: preflight worktree dir: %w", err)
		}
		d, err := u.agents.Get(ctx, crowbarHome, targetProviderID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
		}
		// FINISH THE TURN FIRST. The user can click Switch while the agent is mid-answer, and
		// quitting it there costs the answer twice over: the reply in flight is never written,
		// and — because a CLI killed mid-turn never flushes its native transcript at all — the
		// conversation the next `--resume` names does not exist. That is not a theory; it is
		// the "No conversation found with session ID" the user reported (see awaitTurnComplete).
		//
		// It runs BEFORE every read below, so a switch that is parked is holding nothing but its
		// chat's spawn gate: no aggregate read in progress, no db connection, no half-assembled
		// handoff to go stale while it waits. And it runs before the terminate, so the handoff
		// assembled below contains the turn we waited for.
		if err := u.awaitTurnComplete(ctx, chatID); err != nil {
			return "", err
		}

		priorSessionID, leftAt, err := u.resumableConversation(ctx, chat, targetProviderID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: resumable conversation: %w", err)
		}
		resuming := priorSessionID != ""

		// Read-BEFORE-terminate: the ledger is built from hooks and is already on disk, so
		// assembling the handoff never depends on the outgoing CLI still being alive — and
		// doing it FIRST means a failure here aborts the switch with nothing destroyed,
		// rather than leaving the chat with its old CLI killed and the new one spawned with
		// an EMPTY handoff.
		//
		// A provider resumed into its OWN conversation already holds every turn up to the
		// moment it was switched out, so it is handed only the gap. Replaying the whole
		// ledger to it would duplicate its own history back at it — noise that dilutes the
		// very turns it is meant to notice. A provider new to this chat has no history at
		// all, so it gets the whole conversation.
		conversation, err := u.assembleConversation(ctx, chatID, resuming, leftAt)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
		}

		// How much of the record is NEW to the provider being resumed. It is what
		// the pointer message uses to ask for exactly the gap rather than for the
		// whole conversation this CLI was already handed once.
		gapTurns := 0
		if resuming {
			gap, gapErr := u.activity.TurnsSince(ctx, chatID, leftAt)
			if gapErr != nil {
				return "", fmt.Errorf("agent: switch provider: measure handoff gap: %w", gapErr)
			}
			gapTurns = len(gap)
		}

		retry, err := u.displaceForSwitch(ctx, chat)
		if err != nil {
			return "", err
		}
		if retry {
			continue
		}

		// Resume arg must be split into separate argv tokens: exec.Command does NOT split
		// a string on whitespace, so a whole "--resume {id}" template handed to a single
		// pass_arg would become one literal argument.
		var resumeSteps []engineagents.InjectStep
		if resuming {
			resumeSteps = resumeInjectionSteps(d, priorSessionID)
		}

		// Resume args go first so codex's `resume <id>` subcommand precedes any positional
		// context; order is irrelevant for claude's flag pair.
		return u.spawnRunner(
			ctx, chatID, chat.WorkspaceID, targetProviderID,
			"", resumeSteps, nil, conversation, gapTurns, resuming, priorSessionID, false, "",
		)
	}
}

// displaceForSwitch runs the final busy checks and quits the outgoing CLI, all
// under the turn-start interlock so they are ONE atomic section with respect to
// the hooks that could invalidate them.
//
// retry=true means the switch must go round again rather than proceed: something
// became busy after the first wait, and the handoff assembled from the older
// record would now be missing a turn. Nothing has been destroyed in that case —
// the outgoing CLI is untouched until the very last step.
func (u *Usecase) displaceForSwitch(
	ctx context.Context,
	chat domain.AgentChat,
) (bool, error) {
	unlockTurnStart := u.turnStarts.lock(chat.ID)
	defer unlockTurnStart()

	if err := u.requireNoPendingPromptDelivery(ctx, chat); err != nil {
		return false, err
	}
	if len(u.turns.inflight(chat.ID)) > 0 {
		// A prompt began after the first wait. Let its hook finish, then rebuild the
		// handoff from the now-newer record before trying again.
		return true, nil
	}
	working, err := u.chatWorking(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: final chat work check: %w", err)
	}
	if working {
		// A turn_stop may have handed work to the background after the first await
		// released its runner-scoped turn. Keep the outgoing TUI alive until a later
		// hook authoritatively restates the async-work level as zero.
		return true, nil
	}
	if err := u.quitOutgoingCLI(ctx, chat.ID); err != nil {
		return false, err
	}
	return false, nil
}

// awaitTurnComplete blocks until nothing is mid-turn on chatID, and is the barrier every
// provider switch crosses before it quits anything.
//
// WHY IT EXISTS. quitOutgoingCLI's SIGTERM is graceful so the outgoing CLI can flush its
// native transcript on the way out — but a CLI killed MID-TURN never writes a transcript
// at all. The in-flight answer is lost, and the conversation the incoming CLI is then told
// to resume is not there: `claude --resume` dies with "No conversation found with session
// ID", which is exactly the failure the user reported. Gracefully killing a CLI that is
// still talking is not graceful. So we let it finish, and only then quit it.
//
// WHAT IT BLOCKS ON — the turn's REAL completion, and nothing else. The CLI's turn_stop
// hook lands in IngestHook, which closes the channel this is parked on (turnWaits). There
// is no poll, no sleep and no timeout anywhere in it: a timeout would be a guess about how
// long an agent is allowed to think, and the only honest answer is "as long as the user is
// still waiting for it" — which is precisely what ctx already says. A cancelled request
// therefore aborts the switch with NOTHING changed, the same contract quitOutgoingCLI has
// for its own failures.
//
// THE OTHER WAYS A TURN ENDS are covered too, or the wait would be a hang waiting to
// happen: a CLI that dies mid-answer sends no turn_stop (reconcileRunnerExit releases it),
// and one that leaves the chat mid-answer sends its turn_stop somewhere else (the move and
// displace paths release it). All four run OFF THE HOOK PATH OR OFF THE PTY'S EXIT, and
// neither ever takes the chat's spawn gate — which is what makes it safe to hold that gate
// while parked here (chatGate: "It is never taken on the HOOK path").
//
// An idle chat — the common case by far — has no open turn, so this is one map read and a
// return. The happy path pays nothing.
func (u *Usecase) awaitTurnComplete(
	ctx context.Context,
	chatID string,
) error {
	logged := false
	for {
		// Read the runner-scoped turn first. StopTurn publishes its authoritative
		// Working result before completing this registry entry, so an async-work
		// handoff can never appear as the forbidden (no turn, idle) combination.
		turnOpen, turnChanged := u.turns.watch(chatID)
		working, known, workChanged := u.work.observe(chatID)
		if !known {
			var err error
			if working, workChanged, err = u.seedWorkFromProjection(ctx, chatID); err != nil {
				return err
			}
		}
		if !turnOpen && !working {
			return nil
		}

		if !logged {
			slog.InfoContext(ctx, waitingForTurnLog, "chat_id", chatID)
			logged = true
		}
		select {
		case <-turnChanged:
		case <-workChanged:
		case <-ctx.Done():
			return fmt.Errorf("agent: switch provider: waiting for the chat to become idle: %w", ctx.Err())
		}
	}
}

// seedWorkFromProjection answers "is this chat busy" for a chat this process has
// seen no turn command for — normally a settled chat loaded after a boot.
//
// It re-observes the in-memory signal AFTER reading the projection, and that
// order is the point: a hook that completed while the read was in flight wins,
// and its change signal cannot be missed by the caller parking on it.
func (u *Usecase) seedWorkFromProjection(
	ctx context.Context,
	chatID string,
) (bool, <-chan struct{}, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, nil, fmt.Errorf("agent: switch provider: inspect chat work: %w", err)
	}
	current, known, changed := u.work.observe(chatID)
	if known {
		return current, changed, nil
	}
	return chat.Working, changed, nil
}

// chatWorking returns the aggregate result observed by this process whenever one
// exists, falling back to the durable read model only for a chat with no local turn
// transition. The caller uses it while holding the turn-start interlock.
func (u *Usecase) chatWorking(ctx context.Context, chatID string) (bool, error) {
	if working, known, _ := u.work.observe(chatID); known {
		return working, nil
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, err
	}
	if working, known, _ := u.work.observe(chatID); known {
		return working, nil
	}
	return chat.Working, nil
}

// quitOutgoingCLI gracefully quits the CLI currently pointed at chatID, so a
// well-behaved one — Claude Code in particular — gets the chance to flush its native
// transcript before it dies; a hard kill can lose its last pre-switch turn.
//
// It is only ever reached once the chat is NOT mid-turn (awaitTurnComplete, above), which
// is what makes the graceful kill actually graceful: a CLI still writing its answer has no
// transcript to flush.
//
// A DORMANT chat is not an error: there is simply no outgoing CLI to quit. Such a chat
// used to be a dead end (the pane told the user to "switch provider to start a new one"
// while the switch returned ErrNotFound), so reviving a dead chat runs this exact path —
// that is all ResumeChat is.
//
// A failed terminate is SURFACED, not swallowed: if the outgoing CLI still exists but
// could not be killed, proceeding would leave TWO live CLIs in the same worktree, both
// pointed at this chat. The one error the terminal engine returns today
// (ErrSessionNotFound) means the session is already gone — safe, even correct, to
// continue: the alternative would trap a chat unable to ever switch again once its CLI
// exits on its own.
//
// ORDER — and it is the reverse of every other teardown in this file, deliberately.
// Elsewhere we DISPLACE first (record the placement fact, which cannot fail) and kill
// second. Here the kill is allowed to ABORT the whole switch, so displacing first would
// take the CLI off its chat and then leave it there, alive, holding a chat that now
// believes it has nobody — Crowbar lying about its own placement, which is the one thing
// placement is not allowed to do. So: kill first, and displace only once the CLI is
// definitely going away.
//
// Displacing at all is what closes the window an ordering heuristic cannot: the outgoing
// CLI may not have announced its conversation yet (claude takes about a second, and the
// user can click Switch well inside that). If it announces AFTER the incoming CLI has
// started, it would stamp a fresher timestamp than the incoming runner's spawn — and any
// "whoever arrived last holds the chat" rule would hand the chat back to the corpse.
// Unplaced, it announces into nothing and is dropped.
//
// The runner is NOT Exited here. Its PTY's death does that (onExit → reconcileRunnerExit),
// because the PTY is the only thing that knows.
func (u *Usecase) quitOutgoingCLI(
	ctx context.Context,
	chatID string,
) error {
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // dormant: nothing to quit
	}
	if err != nil {
		return fmt.Errorf("agent: switch provider: live runner: %w", err)
	}
	if err := u.term.TerminateGraceful(ctx, live.TerminalSession); err != nil {
		if !errors.Is(err, engineterminal.ErrSessionNotFound) {
			// The CLI is still on its chat, and it stays there: the switch is aborted with
			// nothing changed rather than half-done.
			return fmt.Errorf("agent: switch provider: terminate outgoing terminal: %w", err)
		}
		slog.WarnContext(ctx, "agent: switch provider: outgoing terminal session already gone before terminate; continuing switch",
			"chat_id", chatID, "runner_id", live.ID, "terminal_session_id", live.TerminalSession, "err", err)
	}
	// A failed displace ABORTS the switch, and this is the one teardown where it must: the
	// caller's very next act is to spawn the incoming CLI, so continuing would place a
	// second runner on a chat the first one is still recorded on — the two-live-CLIs state
	// this whole model exists to make unrepresentable. Aborting is cheap here and costs the
	// user nothing they cannot get back: the outgoing CLI is already dead or dying, so the
	// chat simply drops to dormant when its PTY goes, and Resume revives it.
	if err := u.displace(ctx, live); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	return nil
}

// ResumeChat brings a dormant chat back: its CLI exited, or it died with the daemon
// (agent PTYs are command sessions and never survive a restart). "Dormant" is a
// QUERY — no runner points at the chat — not a flag, so there is no state that could
// say "dormant" while a CLI is demonstrably still running.
//
// Everything needed to revive it is in the chat's conversation history: the provider
// that was last here, and the conversation it was in. So a revive IS a switch to that
// provider — the same path, which finds the conversation and resumes into it, leaving
// the CLI exactly where the user left it.
//
// A chat that still has a live runner is returned as-is (no-op): reviving it would
// tear down a perfectly good CLI and spawn a second one on the same conversation. That
// check and the spawn that may follow it are under the chat's spawn gate, so two clicks
// on Resume cannot both conclude "dormant" and both spawn.
func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: resume chat: live runner: %w", err)
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: no conversation to resume: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	return u.switchProviderLocked(ctx, chatID, last.ProviderID)
}

// StopChat gracefully terminates the chat's live agent process and leaves the
// chat DORMANT (the chat entry and its bound vendor conversation are retained) so
// a later reopen resumes the real conversation via the ordinary ResumeChat path.
// It is what closing a chat TAB calls, and it is the exact counterpart of
// ResumeChat: unlike SwitchProvider it does not respawn, and unlike PurgeChat it
// does not delete the chat.
//
// The in-flight turn is aborted BY DESIGN — "close = stop". It deliberately does
// NOT awaitTurnComplete: completed turns are already flushed to the CLI's on-disk
// transcript (that is what makes --resume work), so only the in-progress reply is
// lost, and that is the cost the user chose. This is precisely the state the daemon
// already handles when an agent process dies on its own mid-turn.
//
// It reuses retire — the same displace-then-graceful-terminate teardown every close
// path in this package converges on — rather than quitOutgoingCLI, and the choice is
// deliberate: quitOutgoingCLI is documented "only ever reached once the chat is NOT
// mid-turn", because a switch WAITS for the turn before quitting so its handoff
// carries it. A close does the opposite. retire fits it exactly:
//
//   - DISPLACE FIRST records the placement fact (the runner is off the chat) — a
//     truth we own, not a liveness claim — which drops the chat to the dormant query
//     (LiveRunnerForChat now reports none) and clears any "Working" turn spinner via
//     closeAbandonedTurn, the moment this returns.
//   - TERMINATE SECOND, best-effort. There is no respawn to protect, so the kill-first
//     ordering quitOutgoingCLI needs does not apply here; a failed SIGTERM leaks a
//     process that dies on its own rather than wedging a close the user asked for
//     (the same best-effort contract PurgeChat's teardown has).
//
// The runner is NOT Exited here. Its PTY's death does that (onExit →
// reconcileRunnerExit — "every teardown path in this package converges here through
// the same door"), the identical door a mid-turn crash exits through — so StopChat
// lands the exact end-state the daemon already produces when a CLI dies on its own:
// no live runner on the chat, the chat still present with its bound conversation, and
// no stuck turn.
//
// A chat with no live runner is ALREADY dormant: a clean nil no-op, not an error —
// the same absence-is-the-answer contract as retireChatRunners.
func (u *Usecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason every teardown path takes it: a stop
	// racing a switch or resume must not terminate a runner the other path is mid-way
	// through placing. It is never taken on the hook path, so a CLI still talking as it
	// dies can always reach us.
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // already dormant: there is no live CLI to stop
	}
	if err != nil {
		return fmt.Errorf("agent: stop chat: live runner: %w", err)
	}
	u.retire(ctx, live)
	return nil
}

// resumableConversation picks the conversation targetProviderID should be resumed
// into, and the moment it left (the cut for the "while you were away" gap). Returns
// "" when there is nothing resumable — the provider is new to this chat, or it never
// actually said anything here — and the caller then spawns it fresh with the whole
// conversation instead.
//
// The second case is the subtle one, and it shipped as a bug: A SESSION ID IS NOT A
// CONVERSATION. A vendor CLI reports its session id the instant it starts (our
// session_start hook records it), but only WRITES that conversation once there is at
// least one message. Resuming such an id fails outright — claude dies on startup with
// "No conversation found with session ID: <id>", which is exactly what a user saw
// after opening a chat they had never sent a message in. So the conversation history
// alone is not enough: the LEDGER must show the provider actually said something.
//
// That same ledger read gives us the cut. The moment a provider "left" is the moment
// it last spoke — which is more honest than any timestamp on the conversation row,
// and it degrades gracefully: whatever happened in this chat after that turn is, by
// definition, what it missed.
func (u *Usecase) resumableConversation(
	ctx context.Context,
	chat domain.AgentChat,
	targetProviderID string,
) (sessionID string, leftAt time.Time, err error) {
	convs, err := u.runners.ConversationsForChat(ctx, chat.ID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("conversations: %w", err)
	}
	// Oldest first, so the LAST match is the most recent conversation this provider
	// held in this chat.
	for _, c := range convs {
		if c.ProviderID == targetProviderID {
			sessionID = c.SessionID
		}
	}
	if sessionID == "" {
		return "", time.Time{}, nil
	}

	leftAt, found, err := u.activity.LastTurnForSession(ctx, chat.ID, targetProviderID, sessionID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("last turn for session: %w", err)
	}
	if !found {
		// The CLI reported this conversation id but never recorded a turn under it, so
		// there is no conversation on disk to resume. Spawn fresh.
		slog.InfoContext(ctx, "agent: prior conversation has no recorded turns; spawning fresh instead of resuming",
			"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID)
		return "", time.Time{}, nil
	}
	return sessionID, leftAt, nil
}

// assembleConversation renders the handoff document for a spawning provider:
// gap-only (turns recorded after leftAt) wrapped in HandoffResumeWrapper when it is
// being resumed into its own conversation, the whole record wrapped in
// HandoffWrapper when it is new to the chat. Empty (not an error) when there is
// nothing to hand over — a brand-new chat, or a revive where nothing happened
// while the CLI was gone; the caller then injects no context document at all.
func (u *Usecase) assembleConversation(
	ctx context.Context,
	chatID string,
	resuming bool,
	leftAt time.Time,
) (string, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return "", fmt.Errorf("agent: assemble conversation: chat: %w", err)
	}

	wrapper := config.GetPrompts().HandoffWrapper
	cut := time.Time{}
	if resuming {
		wrapper = config.GetPrompts().HandoffResumeWrapper
		cut = leftAt
	}

	blob, err := u.renderConversation(ctx, chatID, cut)
	if err != nil {
		return "", fmt.Errorf("agent: assemble conversation: %w", err)
	}
	if len(blob) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(wrapper, "{conversation}", string(blob)), nil
}

// composeContext builds the ONE {context} document a spawning CLI is given, out of
// the sections its caller decided on, in the order it handed them over.
//
// One document, not one per concern, because a provider may only have a single such
// channel — codex delivers preamble and handoff through the same
// developer_instructions key, so two independent injections would collide and the
// second would silently win.
//
// ANY section may be empty, and most spawns have at least one that is: a user can
// blank capabilities_instruction in their own config.yaml, a chat that is not a
// thread has no lineage, and a brand-new chat has no conversation. So the sections
// are joined rather than formatted, and an absent one leaves no stray blank line for
// a model to read as a missing section. WHETHER the result is delivered is
// spawnRunner's decision, not this function's: it turns on whether the CLI is being
// resumed, which nothing in the text can say.
func composeContext(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			parts = append(parts, section)
		}
	}
	return strings.Join(parts, "\n\n")
}

// threadContext renders the standing directive a THREAD is spawned with: the ids of
// every chat above it in the Chats panel, nearest parent first, and the instruction
// to read them with get_chat_log.
//
// It hands over a POINTER and never a paste, which is the whole difference between a
// thread and a fork. Nothing is copied at spawn — the ancestors' turns are fetched
// when the agent asks — so a thread reads its parent AS IT STANDS at the moment of
// the question, including everything that parent has said since this spawn. Pasting
// the ledger here would freeze the parent at the instant the thread started and
// quietly turn a live relationship into a snapshot nobody could refresh.
//
// Empty for a chat with no chat ancestors, which is nearly every chat: one at the
// panel root and one merely filed in a folder both inherit nothing, and neither may
// pay a token for the fact. Folders are transparent to the walk (chatlineage.Walk),
// so filing a thread away never changes what it reads.
//
// A lineage that cannot be READ fails the spawn. Getting no answer is not the same
// fact as having no ancestors, and a thread that comes up silently believing itself
// standalone is the exact failure this path exists to prevent — it would then work
// the whole task without the context it was created to continue.
//
// minting says this spawn is CREATING the chat, and skips the read entirely. Not as
// an optimisation: SpawnChat writes the aggregate only AFTER the CLI is live, since
// a pure command cannot fork a process, so at this point the chat has an id and
// nothing else — no row to read a parent off.
//
// It is not a hole, because that path is only ever taken by a chat with no parent
// to read. A chat that is BORN somewhere is minted and placed first and started
// second (agentchatfolder.CreateChat → MintChat, PlaceChat, StartRunner), so it
// arrives here with create=false and resolves its lineage like any other chat —
// which is what lets a new thread know what it continues on its very FIRST session.
// A chat that is MOVED under another later resolves it on every spawn after the
// move: a provider switch, a revive, a reopened tab.
func (u *Usecase) threadContext(
	ctx context.Context,
	chatID string,
	minting bool,
) (string, error) {
	if minting || u.lineage == nil {
		return "", nil
	}
	ancestors, err := u.lineage.Ancestors(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: chat lineage: %w", err)
	}
	if len(ancestors) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", renderLineage(ancestors)), nil
}

// renderLineage lists the ancestor ids one per line, nearest parent first, as the
// order they arrive in already says.
//
// Ids and nothing else. A title would read better and is deliberately absent: a
// title is user-editable and agent-editable prose that can be blank, stale, or the
// same on two chats, whereas the id is the argument get_chat_log actually takes.
// A model given both reaches for the readable one.
func renderLineage(
	ancestors []string,
) string {
	lines := make([]string, 0, len(ancestors))
	for _, id := range ancestors {
		lines = append(lines, "- "+id)
	}
	return strings.Join(lines, "\n")
}

// requireProviderEnabled refuses a provider the user has switched OFF, and is
// the guard both spawn paths consult (see ErrProviderDisabled for why reporting
// the flag was never enough).
//
// A provider with NO stored row is enabled — the zero preference has
// Disabled=false — which is exactly the default ResolveProviders reports, so the
// enforced answer and the displayed one can never disagree.
func (u *Usecase) requireProviderEnabled(
	ctx context.Context,
	providerID string,
) error {
	pref, err := u.providerPrefs.FindByKey(ctx, providerID)
	if err != nil {
		return fmt.Errorf("agent: provider preference %q: %w", providerID, err)
	}
	if pref != nil && pref.Disabled {
		return fmt.Errorf("%w (%q)", ErrProviderDisabled, providerID)
	}
	return nil
}

// providerMCPEnabled reports whether Crowbar's tool surface should be registered
// with this provider, and mirrors requireProviderEnabled: same store, same
// "no row means the default", same negative column so a freshly migrated table
// does not silently switch every provider's tools off.
//
// It has TWO callers, and they are two different questions about one preference.
// spawnRunner asks whether to RENDER the registration into a CLI's argv, once,
// at spawn. agenttools.Deps.ToolAccess asks whether to SERVE a tool call, on
// every call — because the registration a spawn rendered outlives any later
// change of mind, so the spawn-time read alone would make the switch a
// decoration on every chat that was already running. The switch is described to
// the user as a permission; only the per-call read makes that true.
//
// It ANSWERS rather than refuses, which is the whole difference between the two
// switches. Disabling a provider stops the spawn; disabling its tools does not —
// the CLI comes up, its hooks fire and the chat behaves normally, it simply has
// no Crowbar tools. A guard that returned an error here would have made the
// weaker switch the stronger one.
//
// A read failure fails the spawn rather than defaulting either way. It is not
// reachable in practice — requireProviderEnabled has already read the same row
// off the same store a few lines earlier — but guessing at a preference the user
// set is worse than saying the daemon could not read it.
func (u *Usecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	pref, err := u.providerPrefs.FindByKey(ctx, providerID)
	if err != nil {
		return false, fmt.Errorf("agent: provider mcp preference %q: %w", providerID, err)
	}
	return pref == nil || !pref.MCPDisabled, nil
}

// ListProviders enumerates the registered agent providers for the workspace's
// crowbar home (embedded defaults + on-disk overrides), backing GET
// .../agent/providers. workspaceID is only used to resolve crowbar home — the
// descriptor set is global — so any workspace in the same home yields the same list.
func (u *Usecase) ListProviders(
	ctx context.Context,
	workspaceID string,
) ([]engineagents.Agent, error) {
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: worktree dir: %w", err)
	}
	list, err := u.agents.List(ctx, crowbarHome)
	if err != nil {
		return nil, fmt.Errorf("agent: list providers: %w", err)
	}
	return list, nil
}

// ResolveProviders produces the enriched, priority-ordered provider list the
// backend owns (spec §4.4): the descriptor catalog joined with the global
// preference table and the install probe. It takes NO wsId — providers are global,
// so home comes from app config, never a workspace — which is what lets the global
// PUT handler reuse it for its response.
//
// Order: preferenced providers first, by stored Priority; unpreferenced ones
// (no row) appended after them, by descriptor id. A provider with no row is
// enabled by default (the zero AgentProviderPreference has Disabled=false).
func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]dto.AgentProviderDTO, error) {
	home, err := u.home()
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: home: %w", err)
	}
	descs, err := u.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: descriptors: %w", err)
	}
	prefs, err := u.providerPrefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: preferences: %w", err)
	}
	byID := make(map[string]domain.AgentProviderPreference, len(prefs))
	for _, p := range prefs {
		byID[p.ProviderID] = p
	}

	out := make([]dto.AgentProviderDTO, 0, len(descs))
	for _, d := range descs {
		p := byID[d.ID()]
		display := d.Display()
		caps := d.Capabilities()
		out = append(out, dto.AgentProviderDTO{
			ID:           d.ID(),
			DisplayName:  display.Name,
			Icon:         display.Icon,
			Connected:    u.installed(d),
			Enabled:      !p.Disabled,
			MCPEnabled:   !p.MCPDisabled,
			ModelSelect:  caps.ModelSelect,
			EffortSelect: caps.EffortSelect,
			Models:       d.Models(),
			Efforts:      resolveEfforts(d),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, oki := byID[out[i].ID]
		pj, okj := byID[out[j].ID]
		if oki != okj {
			return oki // a preferenced provider sorts before an unpreferenced one
		}
		if oki && pi.Priority != pj.Priority {
			return pi.Priority < pj.Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ReplaceProviderPreferences rewrites the whole global preference table from the
// submitted ordered set (spec §3.2): the array position becomes each provider's
// Priority. It is a full replace — every submitted row is upserted, and any stored
// row whose id is absent from the submission is deleted (reverting that provider to
// its default enabled+appended state). Every id is validated against the descriptor
// catalog first: an unknown id fails the WHOLE write with apperr.ErrInvalidArgument
// (→ 400), so a bad submission never partially applies. It returns the freshly
// resolved list so the client reconciles from server truth with no second fetch.
func (u *Usecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]dto.AgentProviderDTO, error) {
	home, err := u.home()
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: home: %w", err)
	}
	descs, err := u.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: descriptors: %w", err)
	}
	known := make(map[string]struct{}, len(descs))
	for _, d := range descs {
		known[d.ID()] = struct{}{}
	}
	submitted := make(map[string]struct{}, len(prefs))
	for _, p := range prefs {
		if _, ok := known[p.ProviderID]; !ok {
			return nil, fmt.Errorf("agent: replace provider preferences: unknown provider %q: %w",
				p.ProviderID, apperr.ErrInvalidArgument)
		}
		submitted[p.ProviderID] = struct{}{}
	}

	// Delete stored rows the submission omits FIRST, so a provider dropped from the
	// set reverts to default rather than lingering with a stale priority.
	existing, err := u.providerPrefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: list existing: %w", err)
	}
	for _, e := range existing {
		if _, ok := submitted[e.ProviderID]; ok {
			continue
		}
		if err := u.providerPrefs.Delete(ctx, e.ProviderID); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: delete %q: %w", e.ProviderID, err)
		}
	}
	for _, p := range prefs {
		if err := u.providerPrefs.Save(ctx, p); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: save %q: %w", p.ProviderID, err)
		}
	}
	return u.ResolveProviders(ctx)
}

// ListChats returns every AgentChat.
func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.AgentChat, error) {
	return u.chats.ListChats(ctx)
}

// ListChatsByWorkspace returns every AgentChat anchored to workspaceID.
func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.AgentChat, error) {
	return u.chats.ListByWorkspace(ctx, workspaceID)
}

// GetChat returns a single AgentChat by id.
func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.AgentChat, error) {
	return u.chats.GetChat(ctx, id)
}

// LiveRunnerForChat returns the runner currently pointed at chatID. ErrNotFound
// means the chat is DORMANT — a real answer, not a failure. Row-existence is the
// liveness answer, because the row exists exactly while the PTY does.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

// ConversationsForChat returns every conversation the chat has hosted, oldest
// first — the append-only history that replaced the chat's embedded segments.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}
