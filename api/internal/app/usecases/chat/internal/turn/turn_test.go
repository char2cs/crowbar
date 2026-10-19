package turn_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// apiCtx marks ctx as carrying a delivery over the live api connection —
// inflight.WithAPITransport, the SAME marker pumpAPIConn's own IngestHook
// calls carry in production (see ingest.go's channelFor) — for a test feeding
// an api-shaped codex payload (threadId/turn.*/item.*) through IngestHook
// directly, rather than a genuine hooks-relay POST.
func apiCtx(t *testing.T) context.Context {
	t.Helper()
	return inflight.WithAPITransport(t.Context())
}

// recordingRunners is the CLI lifecycle as a hook sees it: three calls, and
// nothing else. A hook may reach the lifecycle only through this port, and never
// through one of its gated doors — a switch holds the spawn gate while parked on
// a turn only this side can release.
type recordingRunners struct {
	liveOn

	mu       sync.Mutex
	sessions []string
}

func (r *recordingRunners) HandleSessionStart(
	_ context.Context,
	runner engineagents.Runner,
	_ engineagents.CanonicalEvent,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, runner.ID)
	return nil
}

func (r *recordingRunners) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sessions...)
}

// A hook that arrives before its runner row exists must be BUFFERED, not
// ingested: ingesting it would read a runner that is not there yet and drop the
// event on the floor.
func TestIngestHook_BuffersWhileTheRunnerIsStartingAndReplaysAfter(t *testing.T) {
	t.Parallel()

	pending := inflight.NewHooks()
	turns := turn.New(turn.Deps{
		PendingHooks: pending,
		Telemetry:    telemetry.New(),
	})
	runners := &recordingRunners{}
	turns.SetRunners(runners)

	require.NoError(t, pending.Register("runner-1"))
	require.NoError(t, turns.IngestHook(t.Context(), "runner-1", "claude", "session_start", []byte(`{}`)),
		"a buffered hook is absorbed, never refused: by the time it arrives the CLI has already acted")
	assert.Empty(t, runners.seen(), "nothing may be applied while the runner row does not exist")

	var replayed []inflight.Hook
	pending.Finish("runner-1", func(h inflight.Hook) { replayed = append(replayed, h) })

	require.Len(t, replayed, 1)
	assert.Equal(t, "session_start", replayed[0].CanonicalEvent)
}

// Placement is the lifecycle's job: a /clear or /resume inside the TUI moves the
// runner and Crowbar is told after the fact. The hook side decides only that a
// move happened.
func TestReplayStartupHook_RoutesASessionStartThroughTheRunnerPort(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Ledger:       emptyLedger(t),
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	runners := &recordingRunners{liveOn: claudeLive}
	turns.SetRunners(runners)

	turns.ReplayStartupHook("runner-1", inflight.Hook{
		Provider:       "claude",
		CanonicalEvent: "session_start",
		RawPayload:     []byte(`{"session_id":"s-1","transcript_path":"/tmp/t.jsonl"}`),
	})

	assert.Equal(t, []string{"runner-1"}, runners.seen())
}

// A chat nobody has reported for is UNKNOWN, not zero: the header shows a context
// gauge off this, and "0% used" is a claim.
func TestTelemetry_IsUnknownUntilAProviderReports(t *testing.T) {
	t.Parallel()

	turns := turn.New(turn.Deps{Telemetry: telemetry.New()})

	_, ok := turns.Telemetry("chat-1")

	assert.False(t, ok)
}

// The screen classifiers are consulted by a SWEEP over live terminals. A sweep
// that failed on a screen it could not classify would stop classifying the ones
// it could, so every unresolvable input is silent rather than an error.
func TestMatchTerminal_IsSilentForEveryUnresolvableInput(t *testing.T) {
	t.Parallel()

	unresolvableHome := turn.New(turn.Deps{
		Agents: engineagents.New(),
		Home:   func() (string, error) { return "", errors.New("no home") },
	})
	_, ok := unresolvableHome.MatchTerminalPrompt(t.Context(), "claude", "❯ 1. Yes, I trust this folder")
	assert.False(t, ok, "an unreadable home is silent")
	_, ok = unresolvableHome.MatchTerminalNotice(t.Context(), "codex", "usage limit reached")
	assert.False(t, ok)

	home := t.TempDir()
	unknownProvider := turn.New(turn.Deps{
		Agents: engineagents.New(),
		Home:   func() (string, error) { return home, nil },
	})
	_, ok = unknownProvider.MatchTerminalPrompt(t.Context(), "telepathy", "anything")
	assert.False(t, ok, "an unknown provider is silent")
	_, ok = unknownProvider.MatchTerminalNotice(t.Context(), "telepathy", "anything")
	assert.False(t, ok)
}

// stubChats is the chat aggregate reduced to a chat in ws-1.
type stubChats struct {
	agentchat.EventStore
}

func (stubChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, WorkspaceID: "ws-1"}, nil
}

// stubWorkspace roots every path under one temp home.
type stubWorkspace struct{ home string }

func (w stubWorkspace) WorktreeDir(
	context.Context, string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	return w.home, "p1", "r1", w.home, nil
}

func (w stubWorkspace) AgentChatsDir(context.Context, string) (string, error) {
	return w.home, nil
}

// liveOn is the live table a hook resolves through: every runner id names a
// runner of provider on chat-1. Every other Runners method panics if reached.
type liveOn struct {
	turn.Runners
	provider string
}

func (l liveOn) RunnerByID(id string) (engineagents.Runner, bool) {
	return engineagents.Runner{ID: id, ProviderID: l.provider, WorkspaceID: "ws-1", CurrentChatID: "chat-1"}, true
}

// TurnRunner holds the runner open: none of these runners is ending.
func (l liveOn) TurnRunner(id string, open func(engineagents.Runner) error) (bool, error) {
	runner, _ := l.RunnerByID(id)
	return true, open(runner)
}

// claudeLive and codexLive are a claude and a codex runner on chat-1; codex's
// descriptor declares an api transport.
var (
	claudeLive = liveOn{provider: "claude"}
	codexLive  = liveOn{provider: "codex"}
)

// bareRunners answers only which runner a hook names.
type bareRunners struct{ liveOn }

// emptyLedger is a ledger no chat has written to.
func emptyLedger(t *testing.T) *ledgerstore.Store {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	return l
}

// codexBoundLedger is chat-1's ledger bound to codex's sessionID, as the CLI's
// announcement or the api driver's own establish leaves it.
func codexBoundLedger(t *testing.T, sessionID string) *ledgerstore.Store {
	t.Helper()
	l := emptyLedger(t)
	row, err := ledger.Assign(ledger.State{}, "codex", sessionID)
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
	return l
}

// codexRunningLedger is codexBoundLedger with a turn running on runner-1.
func codexRunningLedger(t *testing.T, sessionID string) *ledgerstore.Store {
	t.Helper()
	l := codexBoundLedger(t, sessionID)
	row, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: "runner-1"})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
	return l
}

func busy(t *testing.T, l *ledgerstore.Store) bool {
	t.Helper()
	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	return st.Busy()
}

// recordingToolActivity records InvokeTool calls.
type recordingToolActivity struct {
	agentactivity.EventStore
	mu      sync.Mutex
	invoked []string
}

func (a *recordingToolActivity) InvokeTool(_ context.Context, in agentactivity.ToolInput) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.invoked = append(a.invoked, in.ToolID)
	return nil
}

func (a *recordingToolActivity) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.invoked...)
}

func (*recordingToolActivity) Subagents(context.Context, string) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

// A hooks delivery on a chat with no bound session is recorded: nothing to judge it against.
func TestIngestHook_AHooksDeliveryOnAnUnboundChatIsRecorded(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	activity := &recordingToolActivity{}
	turns := turn.New(turn.Deps{
		Ledger:       emptyLedger(t),
		Chats:        stubChats{},
		Activity:     activity,
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	turns.SetRunners(bareRunners{codexLive})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "tool_pre",
		[]byte(`{"session_id":"s1","tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"echo hi"}}`))

	require.NoError(t, err)
	assert.Equal(t, []string{"tool-1"}, activity.seen())
}

// nativeViewRunners answers only ShowingNativeView, for the surfaceGated
// integration tests below.
type nativeViewRunners struct {
	liveOn
	showing bool
}

func (r nativeViewRunners) ShowingNativeView(string) bool { return r.showing }

// TestRegression_SurfaceGatedMessageDeltaSkipsChatWhileTheNativeViewIsShowing
// proves design spec P6b tag 2 end to end: codex.yaml declares message_delta
// surfaces: [chat], so while ShowingNativeView is true (the user is looking
// at codex's own terminal) this delivery must never reach the chat fan-out —
// wiring the callback and asserting it was never called is what tells "gated
// off" apart from "nothing happened to stream" in the assertion below.
func TestRegression_SurfaceGatedMessageDeltaSkipsChatWhileTheNativeViewIsShowing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var delivered bool
	turns := turn.New(turn.Deps{
		Ledger:       emptyLedger(t),
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	turns.SetRunners(nativeViewRunners{liveOn: codexLive, showing: true})
	turns.SetFeed(seam.ChatFeed{MessageDelta: func(string, string, string, string, string, time.Time) { delivered = true }})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "message_delta",
		[]byte(`{"threadId":"s1","itemId":"m1","delta":"hi","turnId":"t1"}`))

	require.NoError(t, err)
	assert.False(t, delivered,
		"codex.yaml gates message_delta to chat: — the native terminal is on screen, so this "+
			"delivery is not worth listening to")
}

// TestRegression_SurfaceGatedMessageDeltaStillFlowsToChat is the admitting
// half of the same mechanism: chat IS the surface in front of the user, so
// the identical delivery must reach the fan-out.
func TestRegression_SurfaceGatedMessageDeltaStillFlowsToChat(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	var delivered bool
	turns := turn.New(turn.Deps{
		Ledger:       emptyLedger(t),
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	turns.SetRunners(nativeViewRunners{liveOn: codexLive, showing: false})
	turns.SetFeed(seam.ChatFeed{MessageDelta: func(string, string, string, string, string, time.Time) { delivered = true }})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "message_delta",
		[]byte(`{"threadId":"s1","itemId":"m1","delta":"hi","turnId":"t1"}`))

	require.NoError(t, err)
	assert.True(t, delivered, "chat is the surface in front of the user, so a chat-gated event must flow")
}

// stubSubagentActivity answers IsSubagentOpen and nothing else — every other
// EventStore method embeds agentactivity.EventStore and panics if reached,
// same convention as liveAPIRunners above: a test wiring this with open:false
// is asserting a chat that never opened a subagent still safely drops (never
// crashes on, never misroutes) a session it does not recognize.
type stubSubagentActivity struct {
	agentactivity.EventStore
	open bool
}

func (s stubSubagentActivity) IsSubagentOpen(context.Context, string, string) (bool, error) {
	return s.open, nil
}

// recordingSubagentActivity is stubSubagentActivity's twin for the POSITIVE
// case: openSessionID IS a subagent this chat already has open, and the
// nested-routing path's own calls are recorded rather than left to panic on
// the embedded nil EventStore.
type recordingSubagentActivity struct {
	agentactivity.EventStore
	openSessionID string

	stoppedID, stoppedAgentType, stoppedMessage string
}

func (r *recordingSubagentActivity) IsSubagentOpen(
	_ context.Context, _, sessionID string,
) (bool, error) {
	return sessionID == r.openSessionID, nil
}

func (r *recordingSubagentActivity) StopSubagent(
	_ context.Context, _, subagentID, agentType, message string, _ time.Time,
) error {
	r.stoppedID, r.stoppedAgentType, r.stoppedMessage = subagentID, agentType, message
	return nil
}

func (r *recordingSubagentActivity) Subagents(
	context.Context, string,
) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

// A child thread's turn cycle arrives on the runner's own connection; in a chat
// that opened no subagent its turn_stop is foreign and must drop, or it closes
// the user's turn while the answer is still being written.
func TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Ledger:       codexBoundLedger(t, "thread-main"),
		Activity:     stubSubagentActivity{open: false},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	turns.SetRunners(bareRunners{codexLive})

	// api-shaped (threadId): a child thread's own turn/completed arrives over
	// the SAME live api connection as the runner's own thread (see this
	// test's own doc comment), never over the hooks relay.
	err := turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(`{"threadId":"thread-child","turn":{"items":[{"type":"agentMessage","text":"the sub-agent's answer"}]}}`))

	require.NoError(t, err,
		"a child thread's turn/completed says nothing about the turn running on the runner's own thread")
}

// TestRegression_AChildThreadsTurnStopClosesTheSubagentItBelongsTo is the
// positive half TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn
// above cannot cover: when the child thread's id IS a subagent this chat
// already opened (see observation.go's openNestedSubagent, which is what
// would have opened it for real off the parent's own spawnAgent tool call),
// its turn_stop must not be dropped as foreign at all — it is what CLOSES
// that subagent and records its own final reply, the durable nested
// transcript this whole mechanism exists to build.
func TestRegression_AChildThreadsTurnStopClosesTheSubagentItBelongsTo(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	activity := &recordingSubagentActivity{openSessionID: "thread-child"}
	turns := turn.New(turn.Deps{
		Ledger:       codexBoundLedger(t, "thread-main"),
		Activity:     activity,
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	turns.SetRunners(bareRunners{codexLive})

	// api-shaped (threadId/turn.items[type=agentMessage].text): the same
	// live-connection shape as the sibling test above — a child thread's own
	// turn_stop never arrives over the hooks relay.
	err := turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(`{"threadId":"thread-child","turn":{"items":[{"type":"agentMessage","text":"the sub-agent's answer"}]}}`))

	require.NoError(t, err)
	assert.Equal(t, "thread-child", activity.stoppedID,
		"the child thread's own turn_stop must close ITS subagent by id, not be dropped as foreign")
	assert.Equal(t, "the sub-agent's answer", activity.stoppedMessage,
		"the subagent's own final reply must be captured, not discarded")
}

// TestRegression_AChildThreadIsForeignToTheThreadTheDriverOpened: a fresh codex
// api chat binds the thread its own driver opened, so a subagent's thread on the
// same connection is foreign even though nothing registered the subagent.
//
// Chats/Conversations are deliberately wired to nothing: a child thread's
// turn/completed that is NOT dropped reaches closeAssistantTurn and panics on a
// nil activity port, so "drops" is asserted by this call returning at all.
func TestRegression_AChildThreadIsForeignToTheThreadTheDriverOpened(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	l := codexRunningLedger(t, "thread-main")
	turns := turn.New(turn.Deps{
		Chats:        stubChats{},
		Ledger:       l,
		Activity:     stubSubagentActivity{open: false},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		// Nothing streams here: a regression must not idle out the default wait.
		MessageAwaitTimeout: time.Millisecond,
	})
	turns.SetRunners(bareRunners{codexLive})

	require.NoError(t, turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(`{"threadId":"thread-child","turn":{"items":[{"type":"agentMessage","text":"seed"}]}}`)),
		"the subagent's own reply must never be recorded as this chat's assistant message")
	assert.True(t, busy(t, l), "and its turn/completed leaves the chat's turn running")
}

// An end that closes no turn is still written, and the live runner's last
// end, late or not, restates its background work.
func TestTurnStop_ALateEndIsWrittenAndRestatesBackgroundWork(t *testing.T) {
	t.Parallel()

	l := emptyLedger(t)
	_, _, err := l.Append(t.Context(), "chat-1",
		ledgerRow(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{RunnerID: "runner-1", Provider: "claude"}))
	require.NoError(t, err)
	turns := claudeTurns(t, l)
	stop := func(tasks string) {
		t.Helper()
		require.NoError(t, turns.IngestHook(t.Context(), "runner-1", "claude", "turn_stop",
			[]byte(`{"session_id":"s-1","last_assistant_message":"","background_tasks":[`+tasks+`]}`)))
	}

	stop(`{"id":"b1","type":"subagent","status":"running"}`)
	assert.True(t, chatState(t, l).Busy(), "a late end restating background work keeps the chat busy")
	stop("")
	assert.False(t, chatState(t, l).Busy(), "and the next one restating none clears it")

	rows, err := l.Read(t.Context(), "chat-1", 1, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2, "both late ends are written")
	_, hasTurn := chatState(t, l).LastTurn()
	assert.False(t, hasTurn, "and neither opened a turn")
}
