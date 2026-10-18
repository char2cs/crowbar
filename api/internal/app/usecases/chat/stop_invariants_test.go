package chat_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Invariant A1 (spec §7-A), the interruption half: after StopChat returns,
// exactly one "stopped" interruption exists if a turn was open, and none if
// the chat was idle. These run against the REAL turn ingress, so a teardown
// path that completes the in-flight turn before the interruption is recorded
// (displace does exactly that) cannot hide behind a spy.

func stoppedInterruptions(t *testing.T, f testFixture, chatID string) int {
	t.Helper()
	f.wait()
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	n := 0
	for _, in := range ints {
		if in.Kind == engineagents.InterruptStopped {
			n++
		}
	}
	return n
}

func TestInvariantA1_StopMidTurnOnTheRetirePathRecordsExactlyOneStop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID),
		"a Stop that retires a mid-turn CLI must leave exactly one Interrupted divider")
}

func TestInvariantA1_StopOnAnIdleChatRecordsNoStop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "done")
	require.False(t, f.chat(t, chatID).Working, "precondition: the chat is idle")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Zero(t, stoppedInterruptions(t, f, chatID), "closing an idle chat interrupts nothing")
}

func TestInvariantA1_ASecondStopRecordsNothingMore(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID))
}

func TestInvariantA1_AForcedSwitchRecordsExactlyOneStop(t *testing.T) {
	f := newFixture(t).withSwitchGrace(t, 10*time.Millisecond)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID),
		"forcing the outgoing turn is a Stop, and must say so exactly once")
}

// Invariant A2: StopChat completes within a bound while a switch is in flight.
// The switch below parks on a turn that never ends, with an hour's grace
// period; before the gate became preemptible Stop queued behind it for the
// whole of that hour.
func TestInvariantA2_StopPreemptsASwitchParkedOnTheTurn(t *testing.T) {
	f := newFixture(t).withSwitchGrace(t, time.Hour)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	parked := parkedOnTurn(t)
	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()
	select {
	case <-parked:
	case r := <-done:
		t.Fatalf("the switch returned without parking: %+v", r)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- f.usecase.StopChat(f.ctx, chatID) }()

	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("StopChat queued behind a parked switch instead of preempting it")
	}

	got := <-done
	require.ErrorIs(t, got.err, agentusecase.ErrStopped, "the preempted switch reports that Stop won")
	assert.Empty(t, got.runnerID)

	_, err := f.liveRunnerFor(t, chatID)
	require.Error(t, err, "after Stop the chat is dormant: the preempted switch spawned nothing")
	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID))
}

// endHooks runs each hook once, from inside the chat store call where a turn
// closes: stop from StopTurn (a turn_stop), abandon from AbandonTurn (a
// runner's end; after places it once the abandon commits). read runs from the
// first GetChat of delivery readBy's ingest, which holds its runner's hook gate.
type endHooks struct {
	agentchat.EventStore
	mu                  sync.Mutex
	stop, abandon, read func()
	after               bool
	readBy              string
}

func (s *endHooks) arm(slot *func(), hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*slot = hook
}

func (s *endHooks) take(slot *func()) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	hook := *slot
	*slot = nil
	return hook
}

func (s *endHooks) GetChat(ctx context.Context, id string) (domain.Chat, error) {
	s.mu.Lock()
	hook := s.read
	if hook != nil && inflight.DeliveryID(ctx) == s.readBy {
		s.read = nil
	} else {
		hook = nil
	}
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return s.EventStore.GetChat(ctx, id)
}

func (s *endHooks) StopTurn(ctx context.Context, chatID string, now time.Time, asyncWork int) (domain.Chat, error) {
	if hook := s.take(&s.stop); hook != nil {
		hook()
	}
	return s.EventStore.StopTurn(ctx, chatID, now, asyncWork)
}

func (s *endHooks) AbandonTurn(ctx context.Context, chatID string, now time.Time) (domain.Chat, error) {
	hook := s.take(&s.abandon)
	if hook != nil && !s.after {
		hook()
	}
	c, err := s.EventStore.AbandonTurn(ctx, chatID, now)
	if hook != nil && s.after {
		hook()
	}
	return c, err
}

func newEndHooksFixture(t *testing.T, store *endHooks) testFixture {
	t.Helper()
	return newFixtureUsing(t, func(real agentchat.EventStore) agentchat.EventStore {
		store.EventStore = real
		return store
	}, "")
}

// A CLI acknowledging a prompt as its runner ends — Stop, or the CLI dying on
// its own — opens no turn on it: nothing would ever close that turn, and every
// later resume, switch and send would wait on it.
func TestEndingRunner_APromptItAcknowledgesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after bool
		end   func(t *testing.T, f testFixture, chatID string, live engineagents.Runner)
	}{
		{"stop, before the abandon", false, stopChat},
		{"stop, after the abandon", true, stopChat},
		{"crash, before the abandon", false, crashRunner},
		{"crash, after the abandon", true, crashRunner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &endHooks{after: tc.after}
			f := newEndHooksFixture(t, store)
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-ending")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)
			store.arm(&store.abandon, func() {
				ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
				defer cancel()
				payload := mustJSON(t, map[string]any{"prompt": "step"})
				assert.NoError(t, f.usecase.IngestHook(ctx, runnerID, "claude", "user_prompt", payload))
			})

			tc.end(t, f, chatID, live)

			assertEndedAndResumable(t, f, chatID)
		})
	}
}

// A steered prompt the ending runner's last turn_stop carries opens no turn on
// it either: the stop took it off the turn before the end could refuse it.
func TestEndingRunner_ASteerItsTurnStopCarriesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name string
		race func(t *testing.T, f testFixture, store *endHooks, deliveryID string, live engineagents.Runner) func()
	}{
		{"stop", stopWhileTheReplyStreams},
		{"crash", crashAtTheStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &endHooks{}
			f := newEndHooksFixture(t, store)
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-steer")
			prompt(t, f, runnerID, "claude", "think hard about this")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)
			requestID := uuid.NewString()
			steered, err := f.usecase.SubmitPrompt(f.ctx, chatID, "and this too", requestID, "", nil)
			require.NoError(t, err)
			require.Equal(t, runnerID, steered.RunnerID, "precondition: the prompt is steered into the running turn")
			deliveryID := uuid.NewString()
			ended := tc.race(t, f, store, deliveryID, live)

			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			require.NoError(t, f.usecase.IngestHookDelivery(ctx, deliveryID, runnerID, "claude", "turn_stop",
				mustJSON(t, map[string]any{"last_assistant_message": "done"})))
			ended()

			assert.Equal(t, ledger.PromptStatusSettled, receipt(t, f, chatID, requestID).Status,
				"a steer its ending runner never took is refused, so the client keeps its text")
			assertEndedAndResumable(t, f, chatID)
		})
	}
}

// stopWhileTheReplyStreams stops the chat while the turn_stop waits, out of its
// hook gate, for its reply to stream: Stop starts once the ingest holds that
// gate, so after it took the steer, and the stream opens as the end is recorded.
func stopWhileTheReplyStreams(
	t *testing.T, f testFixture, store *endHooks, deliveryID string, live engineagents.Runner,
) func() {
	t.Helper()
	agentusecase.SetMessageAwaitTimeout(f.usecase.own, time.Minute)
	stopped := make(chan error, 1)
	store.mu.Lock()
	store.readBy = deliveryID
	store.mu.Unlock()
	store.arm(&store.read, func() {
		go func() { stopped <- f.usecase.StopChat(f.ctx, live.CurrentChatID) }()
	})
	store.arm(&store.abandon, func() {
		ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
		defer cancel()
		assert.NoError(t, f.usecase.IngestHook(ctx, live.ID, "claude", "message_delta",
			deltaHook(t, "m-done", 0, false, "done")))
	})
	return func() {
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("StopChat never returned")
		}
		require.Nil(t, store.take(&store.abandon), "the stop's end never reached its abandon")
	}
}

// crashAtTheStop crashes the CLI inside the turn_stop's StopTurn: after the
// stop took the steer, before it delivers it.
func crashAtTheStop(t *testing.T, f testFixture, store *endHooks, _ string, live engineagents.Runner) func() {
	t.Helper()
	store.arm(&store.stop, func() { crashRunner(t, f, live.CurrentChatID, live) })
	return func() { require.Nil(t, store.take(&store.stop), "the turn_stop never reached its StopTurn") }
}

func assertEndedAndResumable(t *testing.T, f testFixture, chatID string) {
	t.Helper()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.False(t, f.chat(t, chatID).Working, "the ended runner's chat is left working")
	assert.False(t, agentusecase.TurnInFlight(f.usecase.own, chatID), "the ended runner left a turn in flight")
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	resumed, err := f.usecase.ResumeChat(ctx, chatID)
	require.NoError(t, err, "the chat cannot be resumed")
	assert.NotEmpty(t, resumed)
}

func stopChat(t *testing.T, f testFixture, chatID string, _ engineagents.Runner) {
	t.Helper()
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
}

func crashRunner(t *testing.T, f testFixture, _ string, live engineagents.Runner) {
	t.Helper()
	f.term.exit(t, live.TerminalSession)
}

// stopOp is the walk's Stop. S4: it is bounded, and unless a send raced it the
// chat holds no turn once it returns.
func stopOp(t *testing.T, f testFixture, w *sessionWalk) {
	t.Helper()
	stopped := make(chan error, 1)
	go func() { stopped <- f.usecase.StopChat(f.ctx, w.chatID) }()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("S4: Stop did not return")
	}
	if w.rival != "send" {
		assert.False(t, agentusecase.TurnInFlight(f.usecase.own, w.chatID), "S4: a Stop left a turn open")
	}
}

// assertTurnsStartOnTheLiveRunner checks S6: every turn the ledger records
// starting is on the runner then live on the chat, never on one that ended.
func assertTurnsStartOnTheLiveRunner(t *testing.T, f testFixture, chatID string, trail []string) {
	t.Helper()
	rows, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	live := ""
	for _, r := range rows {
		switch r.Kind {
		case ledger.KindRunnerStarted:
			live = decodeRow[ledger.RunnerStarted](t, r).RunnerID
		case ledger.KindRunnerExited:
			if decodeRow[ledger.RunnerExited](t, r).RunnerID == live {
				live = ""
			}
		case ledger.KindTurnStarted:
			started := decodeRow[ledger.TurnStarted](t, r)
			assert.Equal(t, live, started.RunnerID, "S6: a turn started off the live runner, after %v", trail)
		case ledger.KindUserMessage, ledger.KindPromptSettled, ledger.KindSessionBound,
			ledger.KindSessionLost, ledger.KindMarker: // no runner starts or ends, no turn starts
		}
	}
}

func decodeRow[B any](t *testing.T, r ledger.Row) B {
	t.Helper()
	body, err := ledger.DecodeBody[B](r)
	require.NoError(t, err)
	return body
}

// endWalk ends the walk's runner and checks S3 again: a turn left on a runner
// that ended in any round is still in flight, as only its own end closes it.
func endWalk(t *testing.T, f testFixture, w *sessionWalk, trail []string) {
	t.Helper()
	require.NoError(t, f.usecase.StopChat(f.ctx, w.chatID))
	f.wait()
	checkSessionInvariants(t, f, w, append(trail, "stop"))
}
