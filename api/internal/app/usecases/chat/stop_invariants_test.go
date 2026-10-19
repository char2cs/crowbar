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
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID),
		"a Stop that retires a mid-turn CLI must leave exactly one Interrupted divider")
}

func TestInvariantA1_StopOnAnIdleChatRecordsNoStop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "done")
	require.False(t, f.busy(t, chatID), "precondition: the chat is idle")

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

// readHook runs read once, from the first GetChat of delivery readBy's ingest,
// which holds its runner's hook gate.
type readHook struct {
	agentchat.EventStore
	mu     sync.Mutex
	read   func()
	readBy string
}

func (s *readHook) GetChat(ctx context.Context, id string) (domain.Chat, error) {
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

func (s *readHook) arm(deliveryID string, hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readBy, s.read = deliveryID, hook
}

// A CLI acknowledging a prompt as its runner ends — Stop, or the CLI dying on
// its own — opens no turn on it: nothing would ever close that turn, and every
// later resume, switch and send would wait on it.
func TestEndingRunner_APromptItAcknowledgesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(t *testing.T, f testFixture, chatID string, live engineagents.Runner)
	}{
		{"while the stop ends it", func(t *testing.T, f testFixture, chatID string, live engineagents.Runner) {
			t.Helper()
			var once sync.Once
			f.term.duringTerminate = func(string) { once.Do(func() { acknowledge(t, f, live.ID) }) }
			stopChat(t, f, chatID, live)
		}},
		{"after the stop", func(t *testing.T, f testFixture, chatID string, live engineagents.Runner) {
			t.Helper()
			stopChat(t, f, chatID, live)
			acknowledge(t, f, live.ID)
		}},
		{"before the crash", func(t *testing.T, f testFixture, chatID string, live engineagents.Runner) {
			t.Helper()
			acknowledge(t, f, live.ID)
			crashRunner(t, f, chatID, live)
		}},
		{"after the crash", func(t *testing.T, f testFixture, chatID string, live engineagents.Runner) {
			t.Helper()
			crashRunner(t, f, chatID, live)
			acknowledge(t, f, live.ID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-ending")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)

			tc.end(t, f, chatID, live)

			assertEndedAndResumable(t, f, chatID)
		})
	}
}

func acknowledge(t *testing.T, f testFixture, runnerID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	payload := mustJSON(t, map[string]any{"prompt": "step"})
	assert.NoError(t, f.usecase.IngestHook(ctx, runnerID, "claude", "user_prompt", payload))
}

// A steered prompt the ending runner's last turn_stop carries opens no turn on
// it either: the end settled it before the turn_stop could deliver it.
func TestEndingRunner_ASteerItsTurnStopCarriesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name string
		race func(t *testing.T, f testFixture, store *readHook, deliveryID string, live engineagents.Runner) func()
	}{
		{"stop", stopWhileTheReplyStreams},
		{"crash", crashBeforeTheStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &readHook{}
			f := newFixtureUsing(t, func(real agentchat.EventStore) agentchat.EventStore {
				store.EventStore = real
				return store
			}, "").withMessageAwait(t, time.Minute)
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

// stopWhileTheReplyStreams stops the chat once the turn_stop's ingest holds its
// hook gate; the reply streams only after the stop ended the turn, so the
// turn_stop ends after it.
func stopWhileTheReplyStreams(
	t *testing.T, f testFixture, store *readHook, deliveryID string, live engineagents.Runner,
) func() {
	t.Helper()
	stopped := make(chan error, 1)
	streamed := make(chan struct{})
	var once sync.Once
	f.ledger.Subscribe(func(chatID string, rows []ledger.Row, _ ledger.State) {
		for _, r := range rows {
			if chatID == live.CurrentChatID && r.Kind == ledger.KindTurnEnded {
				once.Do(func() {
					go func() {
						defer close(streamed)
						assert.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "message_delta",
							deltaHook(t, "m-done", 0, false, "done")))
					}()
				})
			}
		}
	})
	store.arm(deliveryID, func() {
		go func() { stopped <- f.usecase.StopChat(f.ctx, live.CurrentChatID) }()
	})
	return func() {
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("StopChat never returned")
		}
		<-streamed
	}
}

// crashBeforeTheStop crashes the CLI before its turn_stop is ingested.
func crashBeforeTheStop(t *testing.T, f testFixture, _ *readHook, _ string, live engineagents.Runner) func() {
	t.Helper()
	crashRunner(t, f, live.CurrentChatID, live)
	return func() {}
}

func assertEndedAndResumable(t *testing.T, f testFixture, chatID string) {
	t.Helper()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.False(t, f.busy(t, chatID), "the ended runner's chat is left working")
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
		assert.False(t, f.busy(t, w.chatID), "S4: a Stop left a turn open")
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
			ledger.KindSessionLost, ledger.KindMarker, ledger.KindTurnEnded,
			ledger.KindTurnFailed: // no runner starts or ends, no turn starts
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
