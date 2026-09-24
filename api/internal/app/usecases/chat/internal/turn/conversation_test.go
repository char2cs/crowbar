package turn_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"

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

// recordingChats records whether the USER's turn was closed. StopTurn is the
// one call that folds Working back to false, so "was it called" IS the
// property (A) turns on.
type recordingChats struct {
	agentchat.EventStore
	mu      sync.Mutex
	stopped int
}

func (c *recordingChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, WorkspaceID: "ws-1", Working: true}, nil
}

func (c *recordingChats) StopTurn(
	_ context.Context, chatID string, _ time.Time, _ int,
) (domain.Chat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped++
	return domain.Chat{ID: chatID, WorkspaceID: "ws-1", Working: false}, nil
}

func (c *recordingChats) RestateAsyncWork(
	_ context.Context, chatID string, _ time.Time, asyncWork int,
) (domain.Chat, error) {
	return domain.Chat{ID: chatID, WorkspaceID: "ws-1", Working: asyncWork > 0}, nil
}

func (c *recordingChats) stops() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
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

func (*recordingTranscript) ToolCalls(
	context.Context, string, int64, int,
) ([]domain.ActivityToolCall, error) {
	return nil, nil
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

// quietRunners answers the liveness reads, the one lifecycle call
// closeTurnFromStop makes, and which conversations this runner's own api
// connection produced — nothing else; every other method embeds turn.Runners
// and panics if reached.
type quietRunners struct {
	turn.Runners
	originated map[string]bool
}

func (quietRunners) ConfirmLaunch(string) {}

func (r quietRunners) OriginatedSession(_, sessionID string) bool { return r.originated[sessionID] }

// The frames these tests replay arrive OFF this connection, so it is there by
// construction: namesAnotherConversation needs it to read the originated
// record at all.
func (quietRunners) HasLiveAPIConnection(string) bool { return true }

// The chat is the surface in front of the user, so message_delta's own
// surfaces: [chat] admits the parent's stream rather than gating it off.
func (quietRunners) ShowingNativeView(string) bool { return false }

func (quietRunners) ReconcilePendingPromptFromLedger(context.Context, domain.Chat) error {
	return nil
}

// delegatingTurn is the live sequence: a sessionless codex runner mid-answer
// on its own thread, with a child thread the model spawned running alongside
// it on the SAME connection.
type delegatingTurn struct {
	turns      *turn.Turns
	chats      *recordingChats
	transcript *recordingTranscript
	work       *inflight.Work
}

func newDelegatingTurn(t *testing.T) delegatingTurn {
	t.Helper()

	home := t.TempDir()
	chats := &recordingChats{}
	transcript := &recordingTranscript{}
	work := inflight.NewWork()
	turns := turn.New(turn.Deps{
		Chats:         chats,
		Runners:       sessionlessCodexRunnerStore{},
		Activity:      transcript,
		Agents:        engineagents.New(),
		Workspace:     stubWorkspace{home: home},
		Home:          func() (string, error) { return home, nil },
		PendingHooks:  inflight.NewHooks(),
		InflightTurns: inflight.NewTurns(),
		Telemetry:     telemetry.New(),
		Work:          work,
	})
	// The parent thread is one Crowbar's own driver minted (apidriver's
	// EstablishSession claims it at spawn) — the whole of what tells it from the
	// child threads codex opens for itself on this same connection.
	turns.SetRunners(quietRunners{originated: map[string]bool{parentThread: true}})
	turns.SetFeed(seam.ChatFeed{MessageDelta: func(string, string, string, string, string) {}})

	// The user's turn is running and the chat is lit — the state every
	// assertion below is about. This is what StartTurn set when the prompt
	// went out, mirrored the same way openTurnFromPrompt mirrors it.
	work.Set("chat-1", true)

	// The parent's own stream, opening the message the child's frames below must
	// not close. Order is not load-bearing any more — the parent is ours by
	// cause, not by having spoken first — which is precisely what the reverted
	// learned baseline could not say.
	require.NoError(t, turns.IngestHook(apiCtx(t), "runner-1", "codex", "message_delta",
		[]byte(parentDelta)))

	return delegatingTurn{turns: turns, chats: chats, transcript: transcript, work: work}
}

// working reports the chat's live Working flag, read off the same mirror
// StopTurn writes through (closeTurnFromStop's t.work.Set). No timing: the
// mirror is set synchronously by the ingest call under test.
func (d delegatingTurn) working(t *testing.T) bool {
	t.Helper()
	working, known, _ := d.work.Observe("chat-1")
	require.True(t, known, "the chat's working state must be mirrored at all")
	return working
}

// TestRegression_AChildThreadsTurnCompletedLeavesASessionlessRunnersTurnOpen
// is defect (A), measured live on codex-cli 0.149.1: a security review
// delegated to a sub-agent, the CHILD's turn/completed arrived 83 SECONDS
// before the user's own turn ended, and closeTurnFromStop filed it against
// the user's chat — StopTurn, Working=false, spinner dark, while codex was
// still writing the answer.
//
// The runner here names no conversation, which is why
// TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn (turn_test.go)
// could not catch this: it wires boundCodexRunnerStore, and a bound session
// is exactly the case that always had something to compare against.
func TestRegression_AChildThreadsTurnCompletedLeavesASessionlessRunnersTurnOpen(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "turn_stop",
		[]byte(childCompleted))

	require.NoError(t, err)
	assert.Zero(t, d.chats.stops(),
		"a child thread's turn/completed says nothing about the turn running on the runner's own thread")
	assert.True(t, d.working(t),
		"the user's turn is still running and the spinner must still be lit")
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

// TestRegression_AChildThreadsIdleNeverArmsASessionlessRunnersIdleFuse is the
// other half of the same capture. The child reports
// thread/status/changed(idle) when ITS turn ends, and recordIdle arms a latch
// the terminal-wait sweep treats as "the provider says it is done and nothing
// has closed the turn" — a 5s fuse (termwait.DefaultIdleQuiet) under the
// user's still-running turn, gated on neither OpenWork nor the connection.
func TestRegression_AChildThreadsIdleNeverArmsASessionlessRunnersIdleFuse(t *testing.T) {
	t.Parallel()

	d := newDelegatingTurn(t)

	err := d.turns.IngestHook(apiCtx(t), "runner-1", "codex", "idle",
		[]byte(childIdle))

	require.NoError(t, err)
	_, armed := d.turns.ProviderIdleSince("chat-1")
	assert.False(t, armed,
		"only the runner's OWN thread going idle may arm the fuse under its turn")
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
	assert.Equal(t, 1, d.chats.stops(), "the runner's own turn/completed closes its turn")
	assert.Equal(t, []string{"the parent is still writing the real answer"}, d.transcript.transcript(),
		"and its own answer is what lands in the transcript")
	assert.False(t, d.working(t), "the spinner goes dark on the runner's own close, not the child's")
}
