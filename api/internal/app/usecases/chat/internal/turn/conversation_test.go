package turn_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
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
	parentCompleted = `{"threadId":"` + parentThread + `","turn":{"id":"t1","items":[{"type":"agentMessage","text":"the parent is still writing the real answer"}]}}`

	parentIdle        = `{"threadId":"` + parentThread + `","status":{"type":"idle"}}`
	parentInterrupted = `{"threadId":"` + parentThread + `","turn":{"id":"t1","status":"interrupted","items":[{"type":"agentMessage","text":"the parent's answer"}]}}`
	parentFailed      = `{"threadId":"` + parentThread + `","turn":{"id":"t1","status":"failed","error":{"message":"overloaded"}}}`
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
	ledger   *ledgerstore.Store
	mu       sync.Mutex
	recorded []string
	busyAt   []bool // whether the chat was busy as each answer was recorded
}

func (a *recordingTranscript) IsSubagentOpen(context.Context, string, string) (bool, error) {
	return false, nil
}

func (a *recordingTranscript) OpenTurn(context.Context, agentactivity.TurnInput) error { return nil }

func (a *recordingTranscript) CloseTurn(ctx context.Context, in agentactivity.TurnInput) error {
	st, err := a.ledger.State(ctx, "chat-1")
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recorded = append(a.recorded, in.Text)
	a.busyAt = append(a.busyAt, st.Busy())
	return nil
}

func (*recordingTranscript) Abandon(context.Context, string, time.Time) error { return nil }

func (a *recordingTranscript) busyAtClose() []bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]bool(nil), a.busyAt...)
}

// quietConversations takes every turn the conversation record is handed.
type quietConversations struct{ turn.Conversations }

func (quietConversations) RecordTurn(context.Context, domain.Chat, string, string, string, string, string, string, string) error {
	return nil
}

func (quietConversations) AppendRunnerTurn(context.Context, domain.Chat, string, string, string, string, string, string) error {
	return nil
}

func (quietConversations) RenameChat(context.Context, string, string, string) error { return nil }

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
	home       string
}

func newDelegatingTurn(t *testing.T) delegatingTurn {
	t.Helper()

	home := t.TempDir()
	l := codexBoundLedger(t, parentThread)
	transcript := &recordingTranscript{ledger: l}
	turns := turn.New(turn.Deps{
		Chats:         &recordingChats{},
		Ledger:        l,
		Activity:      transcript,
		Conversations: quietConversations{},
		Agents:        engineagents.New(),
		Workspace:     stubWorkspace{home: home},
		Home:          func() (string, error) { return home, nil },
		Telemetry:     telemetry.New(),
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
	_, err = placeHook(t, turns, codexLive, engineagents.ChannelAPI, "message_delta", []byte(parentDelta))
	require.NoError(t, err)

	return delegatingTurn{turns: turns, transcript: transcript, ledger: l, home: home}
}

// deliverDeclared ingests, in order, each api report codex's descriptor maps
// to a canonical event; the driver resolves nothing else.
func (d delegatingTurn) deliverDeclared(t *testing.T, events ...[2]string) {
	t.Helper()
	codex, err := engineagents.New().Get(t.Context(), d.home, "codex")
	require.NoError(t, err)
	for _, ev := range events {
		if slices.Contains(codex.Capabilities().Observes, ev[0]) {
			_, err := placeHook(t, d.turns, codexLive, engineagents.ChannelAPI, ev[0], []byte(ev[1]))
			require.NoError(t, err)
		}
	}
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

	_, err := placeHook(t, d.turns, codexLive, engineagents.ChannelAPI, "turn_stop",
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

	_, err := placeHook(t, d.turns, codexLive, engineagents.ChannelAPI, "turn_stop",
		[]byte(childCompleted))

	require.NoError(t, err)
	assert.Empty(t, d.transcript.transcript(),
		"a child thread Crowbar never registered still owns its own words; neither its answer "+
			"nor the parent's half-streamed one may be recorded off its turn/completed")
}

// The runner's own thread reports idle IMMEDIATELY BEFORE its turn/completed
// (codex-cli 0.149.1). Fed as the descriptor maps it, the turn ends on
// turn/completed alone: its answer is recorded while the chat is still busy,
// and the provider's own status is what the turn folds to.
func TestRegression_TheRunnersIdleBeforeItsTurnCompletedNeverEndsTheTurn(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		event, completed string
		status           ledger.TurnStatus
	}{
		"interrupted": {"turn_stop", parentInterrupted, ledger.TurnStatusInterrupted},
		"failed":      {"turn_failed", parentFailed, ledger.TurnStatusFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newDelegatingTurn(t)

			d.deliverDeclared(t, [2]string{"idle", parentIdle}, [2]string{c.event, c.completed})

			st, err := d.ledger.State(t.Context(), "chat-1")
			require.NoError(t, err)
			last, ok := st.LastTurn()
			require.True(t, ok)
			assert.Equal(t, c.status, last.Status, "the provider's own close decides how the turn ended")
			assert.Equal(t, []bool{true}, d.transcript.busyAtClose(), "the answer is recorded before the turn ends")
		})
	}
}

// Ends of a turn that already ended are written late: the fold keeps the turn
// as its own close left it.
func TestRegression_AnEndOfAClosedTurnIsWrittenLate(t *testing.T) {
	t.Parallel()
	d := newDelegatingTurn(t)
	d.deliverDeclared(t, [2]string{"turn_stop", parentCompleted})
	closed, err := d.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)

	d.deliverDeclared(t, [2]string{"turn_stop", parentInterrupted}, [2]string{"turn_failed", parentFailed})

	rows, err := d.ledger.Read(t.Context(), "chat-1", closed.Head(), 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindTurnFailed}, []ledger.Kind{rows[0].Kind, rows[1].Kind})
	st, err := d.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	last, _ := st.LastTurn()
	assert.Equal(t, ledger.TurnStatusDone, last.Status, "a late end never re-closes the turn")
	assert.False(t, st.Busy())
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

	_, err := placeHook(t, d.turns, codexLive, engineagents.ChannelAPI, "turn_stop",
		[]byte(parentCompleted))

	require.NoError(t, err)
	assert.Equal(t, []string{"the parent is still writing the real answer"}, d.transcript.transcript(),
		"and its own answer is what lands in the transcript")
	assert.False(t, d.working(t), "the spinner goes dark on the runner's own close, not the child's")
}
