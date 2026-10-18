package chat_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// TestRegression_SwitchInsideTheUnacknowledgedPromptWindowWaitsRatherThanRefusing
// is the 409 reproduced twice on 2026-09-23: a switch sent between SubmitPrompt's
// 200 and the provider's user_prompt hook, while Working still read false. The
// switch waits for the ledger to acknowledge the prompt, then carries it over.
func TestRegression_SwitchInsideTheUnacknowledgedPromptWindowWaitsRatherThanRefusing(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	prompt(t, f, carrier, "claude", "reply with only: SEED")
	turn(t, f, carrier, "claude", "SEED")

	got := awaitSwitch(t, done)
	require.NoError(t, got.err, "the switch completes once the prompt it waited for is acknowledged")
	assertSwitchedToCodex(t, f, chatID, got.runnerID)
	incoming := f.term.calls[f.term.callCount()-1]
	assert.Contains(t, strings.Join(incoming.argv, "\x00"), "SEED",
		"the handoff carries the turn the switch waited for")
}

// A prompt the provider goes idle without taking is settled, and the settle
// releases the parked switch exactly as an acknowledgement would.
func TestRegression_ASwitchParkedOnAPromptGoesAheadOnceThePromptSettles(t *testing.T) {
	f, chatID, carrier, done := switchParkedOnAPendingPrompt(t)

	require.NoError(t, f.usecase.IngestHook(f.ctx, carrier, "claude", "idle", mustJSON(t,
		map[string]any{"message": "Claude is waiting for your input"})))

	got := awaitSwitch(t, done)
	require.NoError(t, got.err)
	assertSwitchedToCodex(t, f, chatID, got.runnerID)
	prompts := crowbarPrompts(t, f, chatID)
	require.Len(t, prompts, 1)
	assert.Equal(t, ledger.ReasonNotAcknowledged, prompts[0].Reason)
}

// Stop, which preempts the spawn gate, is the one thing besides the ledger that
// ends the wait, and it starts and kills nothing on the switch's behalf.
func TestRegression_StopCancelsASwitchParkedOnAPendingPrompt(t *testing.T) {
	f, chatID, _, done := switchParkedOnAPendingPrompt(t)
	spawns := f.term.callCount()

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	got := awaitSwitch(t, done)
	require.ErrorIs(t, got.err, agentusecase.ErrStopped)
	assert.Empty(t, got.runnerID)
	assert.Equal(t, spawns, f.term.callCount(), "the preempted switch spawned nothing")
}

// switchParkedOnAPendingPrompt submits a prompt and starts a switch to codex,
// returning once the switch has parked on that unacknowledged prompt.
func switchParkedOnAPendingPrompt(t *testing.T) (testFixture, string, string, <-chan switchResult) {
	t.Helper()
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "reply with only: SEED", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the prompt is recorded and unacknowledged")
	require.False(t, f.chat(t, chatID).Working, "precondition: Working is set by the hook that has not arrived")
	carrier, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	parked := parkedOn(t, agentusecase.WaitingForPendingPromptLog)
	done := make(chan switchResult, 1)
	go func() {
		id, switchErr := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: switchErr}
	}()
	select {
	case <-parked:
	case got := <-done:
		t.Fatalf("the switch answered %v inside the unacknowledged-prompt window instead of waiting", got.err)
	}
	return f, chatID, carrier.ID, done
}

func awaitSwitch(t *testing.T, done <-chan switchResult) switchResult {
	t.Helper()
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case got := <-done:
		return got
	case <-deadline.Done():
		t.Fatal("the parked switch never finished")
		return switchResult{}
	}
}

func assertSwitchedToCodex(t *testing.T, f testFixture, chatID, switched string) {
	t.Helper()
	f.wait()
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, switched, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
}

// parkedOnTurn is closed the moment a switch parks on an in-flight turn.
func parkedOnTurn(t *testing.T) <-chan struct{} {
	t.Helper()
	return parkedOn(t, agentusecase.WaitingForTurnLog)
}

// parkedOn is closed when the usecase logs message, which it does immediately
// before it blocks: a test asserts at a moment it knows the switch has reached.
func parkedOn(t *testing.T, message string) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	h := &parkHandler{message: message, ch: make(chan struct{})}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type parkHandler struct {
	message string
	once    sync.Once
	ch      chan struct{}
}

func (h *parkHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *parkHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.message) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *parkHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *parkHandler) WithGroup(string) slog.Handler { return h }
