package turn_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The wire frames these tests replay are the api shapes codex.yaml maps, in
// the order a delegating turn produces them: the parent streams, a child
// thread it spawned completes, the parent finishes. threadId is the only
// thing telling the two apart — see testdata/fixtures/codex/
// item_completed.collabAgentToolCall.json, whose own live capture names the
// parent (senderThreadId/threadId) and the spawned child
// (receiverThreadIds[0]) as two different ids on ONE frame.
const (
	parentThread = "01a0822b-76e6-7ef2-939b-4c50ef4b2046"
	childThread  = "01a0822b-a47b-70d1-92c6-081774183c7b"

	parentDelta     = `{"threadId":"` + parentThread + `","itemId":"m1","delta":"the parent is ","turnId":"t1"}`
	childCompleted  = `{"threadId":"` + childThread + `","turn":{"id":"ct","items":[{"type":"agentMessage","text":"the child thread's answer"}]}}`
	childIdle       = `{"threadId":"` + childThread + `","status":{"type":"idle"}}`
	parentCompleted = `{"threadId":"` + parentThread + `","turn":{"id":"t1","items":[{"type":"agentMessage","text":"the parent is still writing the real answer"}]}}`
)

// recordingChats is the chat record the delegating turn runs on.
type recordingChats struct {
	agentchat.EventStore
}

func (c *recordingChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, WorkspaceID: "ws-1"}, nil
}

// recordingTranscript is the chat's durable transcript. CloseTurn is the only
// call that puts assistant text in it, and IsSubagentOpen answers false
// throughout: property (B) is specifically about the child the daemon NEVER
// registered, so routeNestedSubagentEvent must find nothing and the answer
// must still stay out.
type recordingTranscript struct {
	agentactivity.EventStore
	mu       sync.Mutex
	recorded []string
}

func (a *recordingTranscript) IsSubagentOpen(context.Context, string, string) (bool, error) {
	return false, nil
}

func (a *recordingTranscript) OpenTurn(context.Context, agentactivity.TurnInput) error { return nil }

func (a *recordingTranscript) CloseTurn(_ context.Context, in agentactivity.TurnInput) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recorded = append(a.recorded, in.Text)
	return nil
}

func (*recordingTranscript) Subagents(
	context.Context, string,
) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

func (a *recordingTranscript) transcript() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.recorded...)
}

// quietRunners answers the one question message_delta's surfaces: asks;
// every other method embeds turn.Runners and panics if reached.
type quietRunners struct {
	liveOn
}

// The chat is the surface in front of the user, so message_delta's own
// surfaces: [chat] admits the parent's stream rather than gating it off.
func (quietRunners) ShowingNativeView(string) bool { return false }

// delegatingTurn is the live sequence: a fresh codex api runner mid-answer
// on its own thread, with a child thread the model spawned running alongside
// it on the SAME connection.
type delegatingTurn struct {
	turns      *turn.Turns
	transcript *recordingTranscript
	ledger     *ledgerstore.Store
}

func newDelegatingTurn(t *testing.T) delegatingTurn {
	t.Helper()

	home := t.TempDir()
	transcript := &recordingTranscript{}
	l := codexBoundLedger(t, parentThread)
	turns := turn.New(turn.Deps{
		Chats:        &recordingChats{},
		Ledger:       l,
		Activity:     transcript,
		Agents:       engineagents.New(),
		Workspace:    stubWorkspace{home: home},
		Home:         func() (string, error) { return home, nil },
		PendingHooks: inflight.NewHooks(),
		Telemetry:    telemetry.New(),
	})
	// The parent thread is the chat's bound session: the driver's own establish
	// announced it, which is what tells it from the child threads on this connection.
	turns.SetRunners(quietRunners{codexLive})
	turns.SetFeed(seam.ChatFeed{MessageDelta: func(string, string, string, string, string, time.Time) {}})

	// The user's turn is running on the parent thread: the state every
	// assertion below is about.
	started, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{
		RunnerID: "runner-1", ProviderTurnID: "t1",
	})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", started)
	require.NoError(t, err)

	// The parent's own stream, opening the message the child's frames below must
	// not close.
	require.NoError(t, turns.IngestHook(apiCtx(t), "runner-1", "codex", "message_delta",
		[]byte(parentDelta)))

	return delegatingTurn{turns: turns, transcript: transcript, ledger: l}
}

// working is the chat's ledger saying it is busy.
func (d delegatingTurn) working(t *testing.T) bool {
	t.Helper()
	st, err := d.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	return st.Busy()
}

// TestRegression_AChildThreadsTurnCompletedLeavesTheChatsTurnOpen is defect
// (A), measured live on codex-cli 0.149.1: the CHILD's turn/completed arrived
// 83 SECONDS before the user's own turn ended, and closed the user's turn.
func TestRegression_AChildThreadsTurnCompletedLeavesTheChatsTurnOpen(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(childCompleted))

	require.NoError(t, err)
	assert.True(t, d.working(t),
		"a child thread's turn/completed says nothing about the turn running on the runner's own thread")
}

// TestRegression_AnUnregisteredChildThreadsAnswerNeverEntersTheTranscript is
// defect (B). A live pass found sub-agent output interleaved into the main
// transcript as ordinary assistant messages while GET .../activity reported
// subagents: 0 — the daemon had never registered the child, so
// routeNestedSubagentEvent's IsSubagentOpen gate never engaged.
//
// Registration is a fact Crowbar may simply not receive, so correctness must
// not depend on it: IsSubagentOpen answers false throughout here, and the
// child's answer must stay out of the chat's transcript anyway. Note the
// SECOND assertion — with the guard inert this frame also closes the parent's
// still-streaming message, publishing half an answer as though it were whole.
func TestRegression_AnUnregisteredChildThreadsAnswerNeverEntersTheTranscript(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(childCompleted))

	require.NoError(t, err)
	assert.Empty(t, d.transcript.transcript(),
		"a child thread Crowbar never registered still owns its own words; neither its answer "+
			"nor the parent's half-streamed one may be recorded off its turn/completed")
}

// TestRegression_AChildThreadsIdleNeverEndsTheChatsTurn is the other half of
// the same capture: the child reports thread/status/changed(idle) when ITS
// turn ends, and an idle report closes the running turn at once.
func TestRegression_AChildThreadsIdleNeverEndsTheChatsTurn(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "idle",
		[]byte(childIdle))

	require.NoError(t, err)
	assert.True(t, d.working(t), "only the runner's OWN thread going idle ends its turn")
}

// TestTheRunnersOwnTurnStillClosesItsChat is the admitting half all three
// regressions above need to mean anything: the guard they rely on drops
// events by conversation identity, and a guard that dropped the runner's own
// traffic too would leave a chat permanently silent — strictly worse than the
// leak. The parent's own turn/completed must still close the turn, record its
// answer, and darken the spinner.
func TestTheRunnersOwnTurnStillClosesItsChat(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(parentCompleted))

	require.NoError(t, err)
	assert.Equal(t, []string{"the parent is still writing the real answer"}, d.transcript.transcript(),
		"and its own answer is what lands in the transcript")
	assert.False(t, d.working(t), "the spinner goes dark on the runner's own close, not the child's")
}
