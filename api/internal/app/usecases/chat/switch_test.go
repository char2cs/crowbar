package chat_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// TestRegression_SwitchInsideTheUnconfirmedDeliveryWindowWaitsRatherThanRefusing
// is the 409 the user reproduced twice on 2026-09-23, recovered from the dev
// daemon's own access log:
//
//	14:25:13 POST .../chats/3ddc32fd…/prompts  200  675.9ms
//	14:25:13 POST .../chats/3ddc32fd…/switch   409  1.1ms
//	17:25:14.419Z  that chat's journal record -> "accepted"
//
// The prompt journal record was `spawned` for the ~1.5s between SubmitPrompt
// returning 200 and the provider's user_prompt hook landing. Nothing else said
// so: the chat's own Working flag is set BY that hook, so it still read false,
// and the client — which waits for `working == false` before offering a switch
// — switched inside the window and was refused in about a millisecond. Retried
// seconds later, the same request returned 200.
//
// The guard was right that something was in flight. What was wrong is that it
// REFUSED a state the very next line of the same function knows how to WAIT
// for: an unconfirmed delivery is a turn in flight one moment before the turn
// exists.
func TestRegression_SwitchInsideTheUnconfirmedDeliveryWindowWaitsRatherThanRefusing(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// The measured shape: the delivery is committed (the replacement CLI is
	// forked with the prompt in its argv) and unconfirmed.
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "reply with only: SEED", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingDelivery(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the journal holds a committed, unconfirmed delivery")
	require.False(t, f.chat(t, chatID).Working,
		"precondition: and nothing the client can see says so — Working is set by the hook that has not arrived")

	parked := parkedOnPromptDelivery(t)
	done := make(chan switchResult, 1)
	go func() {
		id, switchErr := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: switchErr}
	}()

	select {
	case <-parked:
	case got := <-done:
		t.Fatalf("the switch answered %q inside the unconfirmed-delivery window; "+
			"a chat that reports itself idle must not be refused as busy", got.err)
	}

	// The window closes exactly as it did live: the provider confirms the prompt
	// and then answers it. Both hooks belong to the REPLACEMENT runner
	// SubmitPrompt forked, not the one spawn returned.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	prompt(t, f, live.ID, "claude", "reply with only: SEED")
	turn(t, f, live.ID, "claude", "SEED")

	got := <-done
	require.NoError(t, got.err, "the switch must complete once the delivery it waited for has landed")
	f.wait()

	newLive, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got.runnerID, newLive.ID)
	assert.Equal(t, "codex", newLive.ProviderID)

	// And the waiting was not for nothing: the turn the switch parked on is in
	// the handoff the incoming CLI is launched with.
	incoming := f.term.calls[f.term.callCount()-1]
	assert.Contains(t, strings.Join(incoming.argv, "\x00"), "SEED",
		"the handoff must carry the turn the switch waited for")
}

// parkedOnPromptDelivery returns a channel closed the moment a switch parks on
// a committed-but-unconfirmed prompt delivery. It is parkedOnTurn's twin, for
// the window just before a turn exists: the usecase's own log record, emitted
// immediately before it blocks, so the assertion above is made at a moment the
// test KNOWS the switch has reached rather than one it slept towards.
func parkedOnPromptDelivery(
	t *testing.T,
) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &promptDeliveryParkHandler{ch: make(chan struct{})}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type promptDeliveryParkHandler struct {
	once sync.Once
	ch   chan struct{}
}

func (h *promptDeliveryParkHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *promptDeliveryParkHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, agentusecase.WaitingForPromptDeliveryLog) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *promptDeliveryParkHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

func (h *promptDeliveryParkHandler) WithGroup(_ string) slog.Handler { return h }
