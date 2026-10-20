package turn

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
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
		Activity: raceActivity{},
		Ledger:   newTurnLedger(t),
	})
	turns.SetRunners(raceRunners{})
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
	turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity}

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
	turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity}

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

func TestCloseAssistantTurn_PersistsTheFinalDiffSnapshot(t *testing.T) {
	activity := &recordingActivity{}
	turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity, live: newLiveText()}
	chat := domain.Chat{ID: "c"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "codex"}
	const diff = "diff --git a/file.txt b/file.txt\n+new content"

	turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "turn-1", MessageID: "message-1", Index: 0, Sequenced: true,
			Final: true, Text: "done",
		},
	})
	turns.live.replace(chat.ID, DeltaKindDiff, "turn-1", diff)

	err := turns.closeAssistantTurn(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "turn_stop", TurnID: "turn-1", Message: "done",
	})
	require.NoError(t, err)
	activity.mu.Lock()
	defer activity.mu.Unlock()
	got, ok := activity.turns[assistantTurnID("message-1")]
	require.True(t, ok)
	require.Equal(t, diff, got.Diff)
}

func TestLiveTextReplaceOverwritesThePreviousCompleteDocument(t *testing.T) {
	live := newLiveText()

	require.Equal(t, "diff v1", live.replace("chat-1", DeltaKindDiff, "turn-1", "diff v1"))
	require.Equal(t, "diff v2", live.replace("chat-1", DeltaKindDiff, "turn-1", "diff v2"))
	require.Equal(t, "diff v2", live.byChat["chat-1"][DeltaKindDiff]["turn-1"].parts[0].String())
}
