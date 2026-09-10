package turn

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestMessageStreams_TheSweepAbandonsWhileAHookIsStillStreaming(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	// White-box on purpose: the race is between a hook goroutine folding increments
	// into the stream and the sweep abandoning the message, and the stream is this
	// package's own — owned outright since it is named nowhere else. Reaching it
	// through the public ingress would mean driving whole hook payloads to provoke a
	// data race that is one field away.
	turns := New(Deps{
		Chats:    raceChats{},
		Runners:  raceRunners{},
		Activity: raceActivity{},
		Work:     inflight.NewWork(),
	})
	racer := &streamRacer{
		turns:    turns,
		messages: turns.messages,
		done:     make(chan struct{}),
	}

	var both sync.WaitGroup
	both.Add(2)
	go func() {
		defer both.Done()
		racer.stream()
	}()
	go func() {
		defer both.Done()
		racer.sweep(t)
	}()
	both.Wait()
}

// recordingActivity mirrors the real projection's storage semantics
// (storage.Store.SaveTurn: an upsert keyed by (ChatID, TurnID), clause.OnConflict
// UpdateAll) closely enough for a test to tell "two closes of the SAME message,
// reconciling in place" apart from "two closes minting DIFFERENT ids" — only the
// latter is ever visibly two rows.
type recordingActivity struct {
	agentactivity.EventStore
	mu    sync.Mutex
	turns map[string]agentactivity.TurnInput
}

func (r *recordingActivity) OpenTurn(context.Context, agentactivity.TurnInput) error { return nil }

func (r *recordingActivity) CloseTurn(_ context.Context, in agentactivity.TurnInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.turns == nil {
		r.turns = map[string]agentactivity.TurnInput{}
	}
	r.turns[in.TurnID] = in
	return nil
}

// TestRegression_ATurnStopThatBeatsItsOwnFinalDeltaDoesNotMintADuplicateMessage
// is the bug reported live 2026-08-29: a fresh Claude reply rendered as two
// back-to-back assistant rows holding nearly the same paragraph, confirmed via
// the live DOM (two distinct, fully-persisted sequence numbers) and the
// daemon's own state (turnId msg-hook-<record-id> alongside turnId
// msg-<streamed-id>).
//
// Root cause: closeAssistantTurn treated "nothing open YET" as "nothing ever
// streamed" and minted its own hook-<record-id> message from the turn-closing
// hook's text, while the REAL streamed message — assembled from message_delta,
// its own independently delivered hook, with no ordering guarantee against
// turn_stop's delivery — was still in flight and landed moments later under
// its own id. Two hooks, two deliveries, no total order between them: this is
// provider- and transport-agnostic, not a Claude- or hooks-specific quirk.
func TestRegression_ATurnStopThatBeatsItsOwnFinalDeltaDoesNotMintADuplicateMessage(t *testing.T) {
	activity := &recordingActivity{}
	turns := &Turns{messages: stream.New(), activity: activity}
	turns.SetMessageAwaitTimeout(2 * time.Second)

	chat := domain.Chat{ID: "c"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}
	const finalText = "the bicycle traces its origins to early 19th-century Europe"

	go func() {
		// The real delta, still in flight when turn_stop is processed — the
		// exact shape confirmed live: MessageDisplay landing after Stop.
		turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
			Kind: "message_delta",
			Delta: &engineagents.MessageDelta{
				TurnID: "t", MessageID: "m1", Index: 0, Sequenced: true,
				Final: true, Text: finalText,
			},
		})
	}()

	err := turns.closeAssistantTurn(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "turn_stop", Message: finalText,
	})
	require.NoError(t, err)

	activity.mu.Lock()
	defer activity.mu.Unlock()
	require.Len(t, activity.turns, 1, "exactly one distinct message id must be recorded for this turn")
	_, gotStreamedID := activity.turns["msg-m1"]
	require.True(t, gotStreamedID,
		"the STREAMED message's own id must win — never a synthesized hook-<record-id> fallback racing ahead of it")
}

// TestRegression_ATerminatingHookThatUnderreportsTextNeverTruncatesTheStream
// is the bug reported live 2026-09-01: a full, multi-paragraph reply was
// visible on screen while the turn was still streaming, but once it closed
// under a Stop hook whose own ev.Message carried only the reply's LAST
// paragraph, closeAssistantTurn's reconciliation unconditionally trusted the
// shorter hook text over the longer, already-broadcast streamed text —
// silently deleting the rest of the reply from the persisted transcript
// (and so from every reload, unlike a still-visible-until-refresh bug).
func TestRegression_ATerminatingHookThatUnderreportsTextNeverTruncatesTheStream(t *testing.T) {
	activity := &recordingActivity{}
	turns := &Turns{messages: stream.New(), activity: activity}

	chat := domain.Chat{ID: "c"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}
	const full = "First paragraph, with real content.\n\nSecond paragraph, also real content."
	const truncated = "Second paragraph, also real content."

	turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "t", MessageID: "m1", Index: 0, Sequenced: true,
			Final: true, Text: full,
		},
	})

	err := turns.closeAssistantTurn(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "turn_stop", Message: truncated,
	})
	require.NoError(t, err)

	activity.mu.Lock()
	defer activity.mu.Unlock()
	require.Len(t, activity.turns, 1,
		"keeping the streamed text must not ALSO leave the hook's shorter report recorded a second time "+
			"under a synthesized id — that was the second bug this exact scenario surfaced")
	got, ok := activity.turns["msg-m1"]
	require.True(t, ok)
	require.Equal(t, full, got.Text,
		"the fuller, already-streamed text must win over a shorter terminating-hook report")
}

// TestCloseAssistantTurn_ATerminatingHookThatReportsMoreTextStillWins is the
// PATTERN test above must not break: the reconciliation exists because a
// delta race can genuinely drop increments, leaving the streamed text
// shorter than what the provider actually said — the hook's own fuller
// report is the correct one to trust in that direction.
func TestCloseAssistantTurn_ATerminatingHookThatReportsMoreTextStillWins(t *testing.T) {
	activity := &recordingActivity{}
	turns := &Turns{messages: stream.New(), activity: activity}

	chat := domain.Chat{ID: "c"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}
	const dropped = "First paragraph, with real content."
	const complete = "First paragraph, with real content.\n\nSecond paragraph, also real content."

	turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "t", MessageID: "m1", Index: 0, Sequenced: true,
			Final: true, Text: dropped,
		},
	})

	err := turns.closeAssistantTurn(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "turn_stop", Message: complete,
	})
	require.NoError(t, err)

	activity.mu.Lock()
	defer activity.mu.Unlock()
	require.Len(t, activity.turns, 1)
	got, ok := activity.turns["msg-m1"]
	require.True(t, ok)
	require.Equal(t, complete, got.Text,
		"a hook report that is FULLER than the stream must still win — the reconciliation's original purpose")
}

// The quiet sweep (runner/internal/termwait) abandons a live turn whose message
// has not grown for DefaultMessageQuiet. It reads UnfinishedSince, which used to
// see the ANSWER stream alone — so a provider that streams a paragraph and then
// reasons for longer than that window, with no ledger-open tool call to vouch for
// it, had its turn closed and its spinner darkened while the CLI was still
// generating. Only codex reaches it: it marks no message final, so its messages
// stay unterminated (and the fuse stays armed) for the whole turn.
func TestRegression_LiveTextKeepsAnUnfinishedMessageFromReadingAsAbandoned(t *testing.T) {
	for _, kind := range []string{DeltaKindReasoning, DeltaKindToolOutput} {
		t.Run(kind, func(t *testing.T) {
			turns := New(Deps{})
			stale := time.Now().Add(-90 * time.Second)

			_, ok := turns.messages.Observe(
				"chat-1", "runner-1", "turn-1", "msg-1", 0, false, false, "Looking into it.", stale,
			)
			require.True(t, ok)

			since, ok := turns.UnfinishedSince("chat-1")
			require.True(t, ok)
			require.Equal(t, stale, since,
				"without live text the answer stream is the only clock")

			beforeDelta := time.Now()
			turns.live.observe("chat-1", kind, "block-1", 0, "still going")

			since, ok = turns.UnfinishedSince("chat-1")
			require.True(t, ok)
			require.False(t, since.Before(beforeDelta),
				"the CLI is still speaking, so the quiet window must restart")
		})
	}
}

// Live text is a view of ONE turn: the next turn's sweep must not be told the
// chat is alive by the previous turn's thinking. closeAssistantTurn forgets it.
func TestRegression_ForgettingLiveTextAlsoForgetsItsQuietClock(t *testing.T) {
	turns := New(Deps{})
	stale := time.Now().Add(-90 * time.Second)

	_, ok := turns.messages.Observe(
		"chat-1", "runner-1", "turn-1", "msg-1", 0, false, false, "Looking into it.", stale,
	)
	require.True(t, ok)
	turns.live.observe("chat-1", DeltaKindReasoning, "block-1", 0, "thinking")
	turns.live.forget("chat-1")

	since, ok := turns.UnfinishedSince("chat-1")
	require.True(t, ok)
	require.Equal(t, stale, since,
		"a forgotten turn's thinking must not keep the next one's sweep at bay")
}
