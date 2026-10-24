package chat_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
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
		if in.Kind == ledger.MarkerStopped {
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

// hookMark marks the ctx of one hook a test places, for readHook to find.
type hookMark struct{}

// marked is ctx marked as the hook named mark.
func marked(ctx context.Context, mark string) context.Context {
	return context.WithValue(ctx, hookMark{}, mark)
}

// readHook runs read once, from the first GetChat of the hook marked readBy,
// which holds its chat's lock.
type readHook struct {
	agentchat.EventStore
	mu     sync.Mutex
	read   func()
	readBy string
}

func (s *readHook) GetChat(ctx context.Context, id string) (domain.Chat, error) {
	s.mu.Lock()
	hook := s.read
	if mark, _ := ctx.Value(hookMark{}).(string); hook != nil && mark == s.readBy {
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

func (s *readHook) arm(mark string, hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readBy, s.read = mark, hook
}

// A CLI acknowledging a prompt as its runner ends — Stop, or the CLI dying on
// its own — opens no turn on it: nothing would ever close that turn, and every
// later resume, switch and send would wait on it.
func TestEndingRunner_APromptItAcknowledgesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(t *testing.T, f testFixture, store *readHook, live engineagents.Runner)
	}{
		{"while the stop ends it", func(t *testing.T, f testFixture, _ *readHook, live engineagents.Runner) {
			t.Helper()
			var once sync.Once
			f.term.duringTerminate = func(string) { once.Do(func() { acknowledge(t, f, live.ID, "", nil) }) }
			stopChat(t, f, live.CurrentChatID, live)
		}},
		{"after the stop", func(t *testing.T, f testFixture, _ *readHook, live engineagents.Runner) {
			t.Helper()
			stopChat(t, f, live.CurrentChatID, live)
			acknowledge(t, f, live.ID, "", apperr.ErrNotFound)
		}},
		{"while it is ingested, the CLI dies", func(t *testing.T, f testFixture, store *readHook, live engineagents.Runner) {
			t.Helper()
			mark := uuid.NewString()
			var recorded <-chan struct{}
			store.arm(mark, func() { recorded = crashOffTheHook(t, f, live) })
			acknowledge(t, f, live.ID, mark, nil)
			finished(t, recorded, "the crash's exit")
		}},
		{"once the CLI's death is recorded", func(t *testing.T, f testFixture, _ *readHook, live engineagents.Runner) {
			t.Helper()
			acked := onceExitRecorded(t, f, live, func() { acknowledge(t, f, live.ID, "", apperr.ErrNotFound) })
			crashRunner(t, f, live.CurrentChatID, live)
			finished(t, acked, "the acknowledgement")
		}},
		{"after the crash", func(t *testing.T, f testFixture, _ *readHook, live engineagents.Runner) {
			t.Helper()
			crashRunner(t, f, live.CurrentChatID, live)
			acknowledge(t, f, live.ID, "", apperr.ErrNotFound)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &readHook{}
			f := newFixtureUsing(t, func(real agentchat.EventStore) agentchat.EventStore {
				store.EventStore = real
				return store
			}, "")
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-ending")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)

			tc.end(t, f, store, live)

			assertEndedAndResumable(t, f, chatID)
		})
	}
}

// acknowledge places runnerID's user_prompt as the hook marked mark, and checks
// it gets want: a runner already off the live table refuses it.
func acknowledge(t *testing.T, f testFixture, runnerID, mark string, want error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(marked(f.ctx, mark), 5*time.Second)
	defer cancel()
	payload := mustJSON(t, map[string]any{"prompt": "step"})
	err := f.usecase.IngestHook(ctx, runnerID, "claude", "user_prompt", payload)
	if want == nil {
		assert.NoError(t, err, "a runner still on the live table takes the hook")
		return
	}
	assert.ErrorIs(t, err, want)
}

// A steered prompt the ending runner's last turn_stop carries leaves no turn
// open on it: a crash recorded first settles it and refuses the turn_stop, and
// a Stop or crash queued behind that turn_stop ends the turn it delivered.
func TestEndingRunner_ASteerItsTurnStopCarriesOpensNoTurn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		race    func(t *testing.T, f testFixture, store *readHook, mark string, live engineagents.Runner, stop func())
		want    ledger.PromptStatus
		hookErr error
	}{
		{"stop", stopWhileTheTurnStopHoldsTheChat, ledger.PromptStatusDelivered, nil},
		{"crash", crashWhileTheStopIsIngested, ledger.PromptStatusDelivered, nil},
		{"crash recorded", stopOnceTheCrashIsRecorded, ledger.PromptStatusSettled, apperr.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &readHook{}
			f := newFixtureUsing(t, func(real agentchat.EventStore) agentchat.EventStore {
				store.EventStore = real
				return store
			}, "")
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-steer")
			prompt(t, f, runnerID, "claude", "think hard about this")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)
			requestID := uuid.NewString()
			steered, err := f.usecase.SubmitPrompt(f.ctx, chatID, "and this too", requestID, "", nil)
			require.NoError(t, err)
			require.Equal(t, runnerID, steered.RunnerID, "precondition: the prompt is steered into the running turn")
			mark := uuid.NewString()
			tc.race(t, f, store, mark, live, func() {
				ctx, cancel := context.WithTimeout(marked(f.ctx, mark), 5*time.Second)
				defer cancel()
				assert.ErrorIs(t, f.usecase.IngestHook(ctx, runnerID, "claude", "turn_stop",
					mustJSON(t, map[string]any{"last_assistant_message": ""})), tc.hookErr)
			})

			assert.Equal(t, tc.want, receipt(t, f, chatID, requestID).Status,
				"only a steer its runner took is delivered; the client keeps any other's text")
			assertEndedAndResumable(t, f, chatID)
		})
	}
}

// A turn end the ledger refuses is refused to its CLI, so the steer it would
// carry is never handed over: it stays pending on the runner, and no turn
// opens for it.
func TestSteer_ATurnEndTheLedgerRefusesHandsOverNoSteer(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-steer")
	prompt(t, f, runnerID, "claude", "think hard about this")
	requestID := uuid.NewString()
	steered, err := f.usecase.SubmitPrompt(f.ctx, chatID, "and this too", requestID, "", nil)
	require.NoError(t, err)
	require.Equal(t, runnerID, steered.RunnerID, "precondition: the prompt is steered into the running turn")
	f.refuseRows(t, ledger.KindTurnEnded)

	err = f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "done"}))

	require.ErrorContains(t, err, "log is full")
	got := receipt(t, f, chatID, requestID)
	assert.Equal(t, ledger.PromptStatusPending, got.Status, "the steer was never handed over")
	for _, r := range ledgerRows(t, f, chatID) {
		if r.Kind == ledger.KindTurnStarted {
			assert.NotEqual(t, got.TurnID, r.TurnID, "no turn opened for the steer")
		}
	}
}

// stopWhileTheTurnStopHoldsTheChat stops the chat once the turn_stop holds its
// lock: the Stop queues behind it, so the steer is handed over first.
func stopWhileTheTurnStopHoldsTheChat(
	t *testing.T, f testFixture, store *readHook, mark string, live engineagents.Runner, stop func(),
) {
	t.Helper()
	stopped := make(chan error, 1)
	store.arm(mark, func() { go func() { stopped <- f.usecase.StopChat(f.ctx, live.CurrentChatID) }() })
	stop()
	require.NoError(t, <-stopped)
}

// crashWhileTheStopIsIngested crashes the CLI once the turn_stop holds its
// chat's lock, before it reads the steer it would deliver: the crash's exit
// waits that turn_stop out, as a Stop does.
func crashWhileTheStopIsIngested(
	t *testing.T, f testFixture, store *readHook, mark string, live engineagents.Runner, stop func(),
) {
	t.Helper()
	var recorded <-chan struct{}
	store.arm(mark, func() { recorded = crashOffTheHook(t, f, live) })
	stop()
	require.NotNil(t, recorded, "the turn_stop never read its chat")
	finished(t, recorded, "the crash's exit")
}

// stopOnceTheCrashIsRecorded sends the turn_stop once the CLI's death is in
// the ledger, while the exit's record still holds the chat.
func stopOnceTheCrashIsRecorded(
	t *testing.T, f testFixture, _ *readHook, _ string, live engineagents.Runner, stop func(),
) {
	t.Helper()
	stopped := onceExitRecorded(t, f, live, stop)
	crashRunner(t, f, live.CurrentChatID, live)
	finished(t, stopped, "the turn_stop")
}

// onceExitRecorded starts during from the frame announcing live's exit, while
// the exit's record still holds the chat, so during waits that record out. The
// channel closes once during returns.
func onceExitRecorded(t *testing.T, f testFixture, live engineagents.Runner, during func()) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	var once sync.Once
	f.usecase.snaps.SetPublish(func(fr agentusecase.ChatSnapshotFrame) {
		f.snapFrames.record(fr)
		if fr.RunnerID != live.ID || fr.Kind != "exited" {
			return
		}
		once.Do(func() {
			go func() {
				defer close(done)
				during()
			}()
		})
	})
	return done
}

func finished(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never ran to its end", what)
	}
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

// crashOffTheHook kills live's PTY now and runs its exit callback on its own
// goroutine, as the terminal's reaper does: the hook in flight holds the chat
// that callback records under. The channel closes once the exit is recorded.
func crashOffTheHook(t *testing.T, f testFixture, live engineagents.Runner) <-chan struct{} {
	t.Helper()
	f.term.mu.Lock()
	onExit := f.term.dieLocked(live.TerminalSession)
	f.term.mu.Unlock()
	require.NotNil(t, onExit, "live's PTY had not exited yet")
	recorded := make(chan struct{})
	go func() {
		defer close(recorded)
		onExit(engineterminal.Exit{})
	}()
	return recorded
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
			ledger.KindSessionLost, ledger.KindMarker, ledger.KindTurnEnded, ledger.KindTurnFailed,
			ledger.KindAssistantText, ledger.KindReasoning, ledger.KindToolCall, ledger.KindAsk,
			ledger.KindAskResolved, ledger.KindSubagent, ledger.KindPlan: // no runner starts or ends, no turn starts
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
