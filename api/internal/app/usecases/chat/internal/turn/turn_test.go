package turn_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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
	turn.Runners

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
		Chats:        stubChats{},
		Runners:      stubRunnerStore{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	runners := &recordingRunners{}
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

// ChatWorking falls back to the aggregate while the process-local mirror knows
// nothing, and the mirror wins the moment it does. Assuming idle is what killed a
// CLI still doing background work after its turn ended.
func TestChatWorking_FallsBackToTheAggregateThenPrefersTheMirror(t *testing.T) {
	t.Parallel()

	work := inflight.NewWork()
	turns := turn.New(turn.Deps{Chats: stubChats{working: true}, Work: work})

	working, err := turns.ChatWorking(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.True(t, working, "the aggregate says the chat IS working and the mirror knows nothing")

	work.Set("chat-1", false)
	working, err = turns.ChatWorking(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.False(t, working, "a known mirror state is newer than any aggregate read")
}

// stubChats is the chat aggregate reduced to the one field these tests decide on.
type stubChats struct {
	agentchat.EventStore
	working bool
}

func (s stubChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, WorkspaceID: "ws-1", Working: s.working}, nil
}

// RestateAsyncWork answers restateAsyncWork's own recount call the same way
// StopTurn would in production (event_store.go: RestateAsyncWork is StopTurn
// with Restate:true) — a fixed reply, since these tests assert on what got
// CALLED, not on a chat's own fold across several calls.
func (s stubChats) RestateAsyncWork(
	_ context.Context, chatID string, _ time.Time, asyncWork int,
) (domain.Chat, error) {
	return domain.Chat{ID: chatID, WorkspaceID: "ws-1", Working: asyncWork > 0}, nil
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

// stubRunnerStore answers "which runner is this" and nothing else.
type stubRunnerStore struct {
	agentrunner.EventStore
}

func (stubRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:            id,
		ProviderID:    "claude",
		WorkspaceID:   "ws-1",
		CurrentChatID: "chat-1",
	}, nil
}

// codexRunnerStore is stubRunnerStore for a codex runner instead of claude's —
// TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent needs a provider
// whose descriptor actually declares an api transport.
type codexRunnerStore struct {
	agentrunner.EventStore
}

func (codexRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:            id,
		ProviderID:    "codex",
		WorkspaceID:   "ws-1",
		CurrentChatID: "chat-1",
	}, nil
}

// liveAPIRunners answers only HasLiveAPIConnection/HasDispatchedOverAPI —
// every other Runners method embeds turn.Runners and panics if reached, which
// is deliberate: this test's whole point is that a redundant hooks delivery
// must return before touching any of them. HasDispatchedOverAPI mirrors live:
// this fixture's "live" connection is the one that already reported the SAME
// turn_stop, so it has genuinely dispatched something to be redundant with.
type liveAPIRunners struct {
	turn.Runners
	live bool
	// originated is the conversations this runner's own api driver minted —
	// what namesAnotherConversation judges an api-channel event against. Empty
	// means "this connection opened nothing", the hooks-only shape.
	originated map[string]bool
}

func (r liveAPIRunners) HasLiveAPIConnection(string) bool { return r.live }
func (r liveAPIRunners) HasDispatchedOverAPI(string) bool { return r.live }
func (r liveAPIRunners) OriginatedSession(_, sessionID string) bool {
	return r.originated[sessionID]
}

// TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent guards the bug
// reported live 2026-08-28: while working with codex, some turns went missing
// mid-stream and then all reappeared at once when the turn finished. Every
// api-transport spawn also forks a real, hooks-wired companion PTY on the SAME
// session (attach.go's own "known gap"), and that PTY's hooks fire the
// descriptor's full hook set regardless of what TransportFor declares — so it
// echoes turn_stop a live api connection already reported. This turn_stop hook
// carries a runner_id whose chat this fixture wires no Chats/Activity/
// Conversations for at all: if the redundant delivery is not recognized and
// dropped BEFORE closeTurnFromStop runs, the call panics on a nil port instead
// of returning cleanly.
func TestIngestHook_DropsAHooksDeliveredCopyOfAnAPIOwnedEvent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Runners:      codexRunnerStore{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveAPIRunners{live: true})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "turn_stop",
		[]byte(`{"session_id":"s1","last_assistant_message":"the reply"}`))

	require.NoError(t, err)
}

// liveButUndispatchedAPIRunners reproduces the EXACT zero-writer production
// incident owner:'s own doc comment (spec/owner.go) and ownerDropsThisDelivery
// (ingest.go) record: a spawn hands its opening prompt to the companion PTY,
// the api side "covers" a turn it never carried, and codex's hooks delivery
// of that SAME event is the turn's ONLY record. Unlike liveAPIRunners above
// (live AND dispatched — genuinely redundant, must drop), HasDispatchedOverAPI
// answers false here: the connection is live but has carried nothing of its
// own.
type liveButUndispatchedAPIRunners struct {
	turn.Runners
}

func (liveButUndispatchedAPIRunners) HasLiveAPIConnection(string) bool      { return true }
func (liveButUndispatchedAPIRunners) HasDispatchedOverAPI(string) bool      { return false }
func (liveButUndispatchedAPIRunners) OriginatedSession(string, string) bool { return false }

// recordingToolActivity records InvokeTool calls and answers OpenWork's own
// reads (ToolCalls/Subagents, consulted by tool_pre's restateAsyncWork tail)
// with nothing open, so this fixture has somewhere real to read from instead
// of panicking on a nil embedded port.
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

func (*recordingToolActivity) ToolCalls(context.Context, string, int64, int) ([]domain.ActivityToolCall, error) {
	return nil, nil
}

func (*recordingToolActivity) Subagents(context.Context, string) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

// TestRegression_TheZeroWriterCaseCannotHappen is design spec P6b's own
// required regression: "when the owning channel is live but has NOT been
// dispatched to, the other channel's delivery must still be recorded." This
// drives the FULL IngestHook pipeline (unlike ingest_internal_test.go's
// TestRegression_ALiveButUndispatchedConnectionNeverMakesTheCompanionPTYsHooksRedundant,
// which pins only the boolean guard) with a REAL activity recorder wired in:
// dropping tool_pre here — codex.yaml declares it owner: api — would be the
// data-loss bug itself, not a passing assertion.
func TestRegression_TheZeroWriterCaseCannotHappen(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	activity := &recordingToolActivity{}
	turns := turn.New(turn.Deps{
		Runners:      codexRunnerStore{},
		Chats:        stubChats{},
		Activity:     activity,
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveButUndispatchedAPIRunners{})

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "tool_pre",
		[]byte(`{"session_id":"s1","tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"echo hi"}}`))

	require.NoError(t, err)
	assert.Equal(t, []string{"tool-1"}, activity.seen(),
		"a live-but-undispatched api connection must never make the companion PTY's hooks "+
			"delivery look redundant — this IS the turn's only record")
}

// nativeViewRunners answers only ShowingNativeView, for the surfaceGated
// integration tests below.
type nativeViewRunners struct {
	turn.Runners
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
		Runners:      codexRunnerStore{},
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(nativeViewRunners{showing: true})
	turns.SetMessageDelta(func(string, string, string, string, string) { delivered = true })

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
		Runners:      codexRunnerStore{},
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(nativeViewRunners{showing: false})
	turns.SetMessageDelta(func(string, string, string, string, string) { delivered = true })

	err := turns.IngestHook(t.Context(), "runner-1", "codex", "message_delta",
		[]byte(`{"threadId":"s1","itemId":"m1","delta":"hi","turnId":"t1"}`))

	require.NoError(t, err)
	assert.True(t, delivered, "chat is the surface in front of the user, so a chat-gated event must flow")
}

// boundCodexRunnerStore is a codex runner that is ON a conversation, which is
// what the two regression tests below turn on: the child-thread bug is invisible
// unless the runner actually holds a session to compare an event against.
type boundCodexRunnerStore struct {
	agentrunner.EventStore
	session string
}

func (s boundCodexRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:             id,
		ProviderID:     "codex",
		WorkspaceID:    "ws-1",
		CurrentChatID:  "chat-1",
		CurrentSession: s.session,
	}, nil
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
// nested-routing path's own calls (StopSubagent, and OpenWork's own
// ToolCalls/Subagents reads) are recorded rather than left to panic on the
// embedded nil EventStore.
type recordingSubagentActivity struct {
	agentactivity.EventStore
	openSessionID string

	stoppedID, stoppedAgentType, stoppedMessage string
}

func (r *recordingSubagentActivity) IsSubagentOpen(
	_ context.Context, _ string, sessionID string,
) (bool, error) {
	return sessionID == r.openSessionID, nil
}

func (r *recordingSubagentActivity) StopSubagent(
	_ context.Context, _, subagentID, agentType, message string, _ time.Time,
) error {
	r.stoppedID, r.stoppedAgentType, r.stoppedMessage = subagentID, agentType, message
	return nil
}

func (r *recordingSubagentActivity) ToolCalls(
	context.Context, string, int64, int,
) ([]domain.ActivityToolCall, error) {
	return nil, nil
}

func (r *recordingSubagentActivity) Subagents(
	context.Context, string,
) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

// TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn guards the bug
// reported live 2026-09-11 ("we're still missing that the agent is working...
// it always happens during a security review"). codex pushes a child thread's
// COMPLETE, independent turn cycle down the SAME websocket the runner's own
// thread uses — a collab agent, or the review/compaction/memory threads it
// spawns unbidden. Captured live on a security review that delegated to a
// sub-agent: the child's turn/completed arrived 83 SECONDS before the user's own
// turn ended, and ingest filed it against the user's chat, closing the turn and
// darkening the spinner while codex was still writing the answer.
//
// This chat never opened a subagent (stubSubagentActivity{open: false}), so
// the child thread's own turn_stop is genuinely foreign to it and must still
// drop before reaching closeTurnFromStop — which is why Chats/Conversations
// are still wired to nothing at all: routeNestedSubagentEvent's own
// IsSubagentOpen check is now the first activity touch on this path, but
// finding nothing open must return control to the SAME early drop this test
// always pinned, never fall through to code that needs those ports. See
// TestRegression_AChildThreadsTurnStopClosesTheSubagentItBelongsTo below for
// the OTHER half — a chat that DID open one.
func TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Runners:      boundCodexRunnerStore{session: "thread-main"},
		Activity:     stubSubagentActivity{open: false},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveAPIRunners{live: false})

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
		Runners:      boundCodexRunnerStore{session: "thread-main"},
		Activity:     activity,
		Chats:        stubChats{},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveAPIRunners{live: false})

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

// TestRegression_AChildThreadsIdleNeverArmsThisChatsProviderIdleFuse is the
// other half of the same live capture. The child thread reports
// thread/status/changed(idle) when ITS turn ends, and recordIdle arms a latch
// the terminal-wait sweep treats as "the provider says it is done and nothing
// has closed the turn" — a 5s fuse (termwait.DefaultIdleQuiet) under the user's
// still-running turn, deliberately gated on neither OpenWork nor the live
// connection. Only the runner's own thread going idle may arm it.
//
// idle is not one of handleNestedObservation's handled kinds even for a
// chat's own open subagent (this pass records a subagent's tool calls and
// final reply, not a live idle signal for it), so stubSubagentActivity{open:
// false} here is enough either way — the point pinned is narrower than
// "never open": it is "idle specifically must never arm this chat's own
// fuse," true whether or not the id turns out to belong to a subagent.
func TestRegression_AChildThreadsIdleNeverArmsThisChatsProviderIdleFuse(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	newTurns := func() *turn.Turns {
		turns := turn.New(turn.Deps{
			Chats:        stubChats{working: true},
			Runners:      boundCodexRunnerStore{session: "thread-main"},
			Activity:     stubSubagentActivity{open: false},
			Agents:       engineagents.New(),
			Workspace:    stubWorkspace{home: home},
			Home:         func() (string, error) { return home, nil },
			PendingHooks: inflight.NewHooks(),
			Telemetry:    telemetry.New(),
			Work:         inflight.NewWork(),
		})
		turns.SetRunners(liveAPIRunners{live: false})
		return turns
	}

	foreign := newTurns()
	require.NoError(t, foreign.IngestHook(t.Context(), "runner-1", "codex", "idle",
		[]byte(`{"threadId":"thread-child","status":{"type":"idle"}}`)))
	_, armed := foreign.ProviderIdleSince("chat-1")
	require.False(t, armed,
		"a child thread finishing says nothing about the runner's own turn; arming here lets the 5s sweep abandon a live one")

	own := newTurns()
	require.NoError(t, own.IngestHook(t.Context(), "runner-1", "codex", "idle",
		[]byte(`{"threadId":"thread-main","status":{"type":"idle"}}`)))
	_, armed = own.ProviderIdleSince("chat-1")
	require.True(t, armed,
		"the runner's OWN thread going idle must still arm the latch, or the turn nothing ever closes is unreachable again")
}

// sessionlessCodexRunnerStore is the runner row a FRESH codex api-transport
// chat actually leaves behind: no CurrentSession and no LaunchSessionID.
// applyAPITransport mints the thread id inside the driver's own establish call
// and starts pumpAPIConn only afterwards (apiconn.go), so thread/started never
// reaches HandleSessionStart and nothing ever binds. Measured 2026-09-23 over
// this repo's dev runner store: 30 of 34 codex runners, every one of them a
// chat that was not resumed.
type sessionlessCodexRunnerStore struct {
	agentrunner.EventStore
}

func (sessionlessCodexRunnerStore) Get(_ context.Context, id string) (engineagents.Runner, error) {
	return engineagents.Runner{
		ID:            id,
		ProviderID:    "codex",
		WorkspaceID:   "ws-1",
		CurrentChatID: "chat-1",
	}, nil
}

// TestRegression_AChildThreadIsForeignEvenWhenNothingEverNamedTheRunnersOwnSession
// is the second shape of the 2026-09-23 collab-agents transcript bleed, live in
// chat d4912d6d: three agent identities in one transcript, the subagents' own
// replies ("seed", "0") filed as ordinary top-level assistant messages between
// the main agent's opener and its summary, and GET .../activity reporting
// subagents: 0 — no spawnAgent tool call ever arrived, so nothing registered
// them and the nested routing never engaged.
//
// The launch-session fallback added for the resume case is inert here: this
// runner names NO conversation at all, so the ownership guard had nothing to
// compare against and waved every child thread through. The api channel is
// answered by CAUSE instead — Crowbar's own driver minted thread-main at
// establish and never opened thread-child — which is what the reverted
// "first conversation named wins" heuristic only approximated.
//
// Chats/Conversations are deliberately wired to nothing: a child thread's
// turn/completed that is NOT dropped reaches closeAssistantTurn and panics on a
// nil activity port, so "drops" is asserted by this call returning at all. Same
// convention as TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn.
func TestRegression_AChildThreadIsForeignEvenWhenNothingEverNamedTheRunnersOwnSession(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Chats:        stubChats{working: true},
		Runners:      sessionlessCodexRunnerStore{},
		Activity:     stubSubagentActivity{open: false},
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
		Work:         inflight.NewWork(),
	})
	turns.SetRunners(liveAPIRunners{live: true, originated: map[string]bool{"thread-main": true}})
	// Nothing streams in this fixture, so closeAssistantTurn's delta race has
	// nothing to wait for — without this a REGRESSION would idle 3s before
	// failing. The passing path never reaches it.
	turns.SetMessageAwaitTimeout(0)

	// The runner's own thread speaks first, as it must.
	require.NoError(t, turns.IngestHook(apiCtx(t), "runner-1", "codex", "idle",
		[]byte(`{"threadId":"thread-main","status":{"type":"idle"}}`)))
	_, armed := turns.ProviderIdleSince("chat-1")
	require.True(t, armed,
		"a runner that never bound a session must still act on its OWN thread's events")

	// And only then the subagent codex spawned on the same connection.
	require.NoError(t, turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(`{"threadId":"thread-child","turn":{"items":[{"type":"agentMessage","text":"seed"}]}}`)),
		"the subagent's own reply must never be recorded as this chat's assistant message")
}
