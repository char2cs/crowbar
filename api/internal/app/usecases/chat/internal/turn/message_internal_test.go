package turn

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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
	// refuseNext fails the next close, once.
	refuseNext error
}

func (r *recordingActivity) OpenTurn(context.Context, agentactivity.TurnInput) error { return nil }

func (r *recordingActivity) CloseTurn(_ context.Context, in agentactivity.TurnInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.refuseNext; err != nil {
		r.refuseNext = nil
		return err
	}
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

	require.NoError(t, turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "t", MessageID: "m1", Index: 0, Sequenced: true,
			Final: true, Text: full,
		},
	}))

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

	require.NoError(t, turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "t", MessageID: "m1", Index: 0, Sequenced: true,
			Final: true, Text: dropped,
		},
	}))

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

	require.NoError(t, turns.recordMessageDelta(context.Background(), chat, runner, engineagents.CanonicalEvent{
		Kind: "message_delta",
		Delta: &engineagents.MessageDelta{
			TurnID: "turn-1", MessageID: "message-1", Index: 0, Sequenced: true,
			Final: true, Text: "done",
		},
	}))
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

// texts is every recorded message's text by its turn id.
func (r *recordingActivity) texts() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for id, in := range r.turns {
		out[id] = in.Text
	}
	return out
}

// replies is every recorded message's text, sorted.
func (r *recordingActivity) replies() []string {
	return slices.Sorted(maps.Values(r.texts()))
}

func finalDelta(messageID, text string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{Kind: engineagents.HookMessageDelta, Delta: &engineagents.MessageDelta{
		TurnID: "t", MessageID: messageID, Sequenced: true, Final: true, Text: text,
	}}
}

// A final message whose write failed is still unrecorded: its hook is refused,
// and its turn's close records it.
func TestRecordMessageDelta_AMessageWhoseWriteFailedIsRecordedByItsTurnsClose(t *testing.T) {
	refused := errors.New("log is full")
	activity := &recordingActivity{refuseNext: refused}
	turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity, live: newLiveText()}
	chat := domain.Chat{ID: "c"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}

	require.ErrorIs(t, turns.recordMessageDelta(t.Context(), chat, runner, finalDelta("m1", "FIRST")), refused)
	require.NoError(t, turns.recordMessageDelta(t.Context(), chat, runner, finalDelta("m2", "SECOND")))
	require.NoError(t, turns.closeAssistantTurn(t.Context(), chat, runner, engineagents.CanonicalEvent{Kind: "turn_stop"}))

	assert.Equal(t, map[string]string{"msg-m1": "FIRST", "msg-m2": "SECOND"}, activity.texts())
}

// The bug of 2026-08-29: a reply's turn_stop and its final delta are two hooks
// the chat's lock serialises in either order, and the reply is recorded once
// whichever lands first.
func TestRegression_AReplyIsRecordedOnceWhicheverOfItsHooksLandsFirst(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	for _, deltaFirst := range []bool{true, false} {
		activity := &recordingActivity{}
		turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity, live: newLiveText()}
		chat := domain.Chat{ID: "c"}
		runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}
		stop := func() error {
			return turns.closeAssistantTurn(t.Context(), chat, runner, engineagents.CanonicalEvent{Kind: "turn_stop", Message: reply})
		}
		hooks := []func() error{stop, func() error { return turns.recordMessageDelta(t.Context(), chat, runner, finalDelta("m1", reply)) }}
		if !deltaFirst {
			hooks[0], hooks[1] = hooks[1], hooks[0]
		}

		for _, hook := range hooks {
			require.NoError(t, hook())
		}

		assert.Equal(t, []string{reply}, activity.replies(), "delta first: %v", deltaFirst)
	}
}

// lateReply is a turn closed on reply by its hook, nothing streamed yet, and
// the ids every later delta is shown under.
func lateReply(t *testing.T, reply string) (*Turns, *recordingActivity, *[]string) {
	t.Helper()
	activity := &recordingActivity{}
	turns := &Turns{ledger: newTurnLedger(t), messages: stream.New(), activity: activity, live: newLiveText()}
	live := &[]string{}
	turns.feed.MessageDelta = func(_, _, id, _, _ string, _ time.Time) { *live = append(*live, id) }
	stop := engineagents.CanonicalEvent{Kind: "turn_stop", Message: reply}
	require.NoError(t, turns.closeAssistantTurn(t.Context(), domain.Chat{ID: "c"}, lateRunner, stop))
	return turns, activity, live
}

var lateRunner = engineagents.Runner{ID: "runner-1", ProviderID: "claude"}

// A late delta that joins its turn's reply is shown under that reply's id from
// its first increment: the client drops a live bubble once a row of its id lands.
func TestRecordMessageDelta_ALateDeltaThatJoinsItsReplyIsShownUnderItsID(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	turns, activity, live := lateReply(t, reply)
	head := finalDelta("m1", reply[:11])
	head.Delta.Final = false
	tail := finalDelta("m1", reply[11:])
	tail.Delta.Index = 1

	require.NoError(t, turns.recordMessageDelta(t.Context(), domain.Chat{ID: "c"}, lateRunner, head))
	require.NoError(t, turns.recordMessageDelta(t.Context(), domain.Chat{ID: "c"}, lateRunner, tail))

	recorded := slices.Collect(maps.Keys(activity.texts()))
	require.Len(t, recorded, 1, "one reply")
	require.Len(t, *live, 2)
	for _, id := range *live {
		assert.Equal(t, recorded[0], assistantTurnID(id))
	}
}

// A late delta joins its turn's reply only as that reply streamed late: a
// subagent's message, or another text, is its own message under its own id.
func TestRecordMessageDelta_ALateDeltaJoinsOnlyItsOwnReply(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	subagent := finalDelta("m1", reply)
	subagent.Subagent = &engineagents.SubagentEvent{ID: "a1"}
	for name, late := range map[string]engineagents.CanonicalEvent{
		"a subagent's": subagent,
		"another text": finalDelta("m1", "an answer to another prompt"),
	} {
		t.Run(name, func(t *testing.T) {
			turns, activity, live := lateReply(t, reply)

			require.NoError(t, turns.recordMessageDelta(t.Context(), domain.Chat{ID: "c"}, lateRunner, late))

			assert.Equal(t, []string{"m1"}, *live)
			texts := activity.texts()
			assert.Len(t, texts, 2, "the reply its turn closed on is left as it was")
			assert.Equal(t, late.Delta.Text, texts[assistantTurnID("m1")])
		})
	}
}

// A late stream whose head read as the reply but whose words turn out another's
// is its own message: a join never crosses messages.
func TestRecordMessageDelta_ALateStreamThatTurnsOutAnotherMessageIsItsOwn(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	turns, activity, _ := lateReply(t, reply)
	head := finalDelta("m1", "the ")
	head.Delta.Final = false
	tail := finalDelta("m1", "cat sat on the mat")
	tail.Delta.Index = 1

	require.NoError(t, turns.recordMessageDelta(t.Context(), domain.Chat{ID: "c"}, lateRunner, head))
	require.NoError(t, turns.recordMessageDelta(t.Context(), domain.Chat{ID: "c"}, lateRunner, tail))

	assert.Equal(t, []string{reply, "the cat sat on the mat"}, activity.replies())
}

// A reply's late stream left unfinished never records the reply into the next
// turn: the reply keeps its place and its text, and owns none of that turn's tools.
func TestCloseAssistantTurn_AnOldReplyIsNeverRecordedIntoTheNextTurn(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	for name, late := range map[string]string{
		"a late stream that adds nothing": reply[:22],
		"a late stream past the reply":    reply + ", and spread from there",
	} {
		t.Run(name, func(t *testing.T) {
			turns, activity, settle := newActivityTurns(t)
			first := lateStreamAfterAReply(t, turns, reply, late)
			settle()
			before := activityTurn(t, activity, first)

			ended, err := ledger.NewRow(ledger.KindTurnEnded, "T-runner-1", "", ledger.TurnEnded{Status: "done", RunnerID: lateRunner.ID})
			require.NoError(t, err)
			next, err := ledger.NewRow(ledger.KindTurnStarted, "T-2", "", ledger.TurnStarted{RunnerID: lateRunner.ID})
			require.NoError(t, err)
			appendTurnRows(t, turns.ledger, ended, next)
			turns.openAssistantTurn(t.Context(), lateChat, lateRunner)
			require.NoError(t, activity.InvokeTool(t.Context(), agentactivity.ToolInput{ChatID: lateChat.ID, ToolID: "tool-2", Name: "Read", Now: time.Now()}))
			stop := engineagents.CanonicalEvent{Kind: "turn_stop", Message: "the next reply"}
			require.NoError(t, turns.closeAssistantTurn(t.Context(), lateChat, lateRunner, stop))
			settle()

			after := activityTurn(t, activity, first)
			assert.Equal(t, before.DisplayOrder, after.DisplayOrder)
			assert.Equal(t, reply, after.Text, "the turn's recorded answer stands")
			tools, err := activity.ToolCalls(t.Context(), lateChat.ID, 0, 10)
			require.NoError(t, err)
			require.Len(t, tools, 1)
			assert.Equal(t, activityTurnByText(t, activity, "the next reply").ID, tools[0].TurnID)
		})
	}
}

// A late stream that adds nothing to its reply writes nothing when its runner
// ends: the reply keeps its place, and no second row is made of it.
func TestAbandonMessageForRunner_ALateStreamThatAddsNothingLeavesItsReplyInPlace(t *testing.T) {
	const reply = "the bicycle traces its origins to early 19th-century Europe"
	turns, activity, settle := newActivityTurns(t)
	first := lateStreamAfterAReply(t, turns, reply, reply[:22])
	settle()
	before := activityTurn(t, activity, first)

	_, err := turns.AbandonMessageForRunner(t.Context(), lateChat.ID, lateRunner)
	require.NoError(t, err)
	settle()

	assert.Equal(t, before.DisplayOrder, activityTurn(t, activity, first).DisplayOrder)
	all, err := activity.TurnsSince(t.Context(), lateChat.ID, time.Time{})
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

var lateChat = domain.Chat{ID: "chat-1"}

// lateStreamAfterAReply runs a turn that closes on reply, nothing streamed, then
// opens late as that reply's unfinished late stream. It returns the reply's row id.
func lateStreamAfterAReply(t *testing.T, turns *Turns, reply, late string) string {
	t.Helper()
	appendTurnRows(t, turns.ledger, turnRow(t, lateRunner.ID))
	turns.openAssistantTurn(t.Context(), lateChat, lateRunner)
	stop := engineagents.CanonicalEvent{Kind: "turn_stop", Message: reply}
	require.NoError(t, turns.closeAssistantTurn(t.Context(), lateChat, lateRunner, stop))
	answer, ok := turns.messages.Answer(lateChat.ID, lateRunner.ID)
	require.True(t, ok)
	head := finalDelta("m1", late)
	head.Delta.Final = false
	require.NoError(t, turns.recordMessageDelta(t.Context(), lateChat, lateRunner, head))
	return assistantTurnID(answer.MessageID)
}

func activityTurn(t *testing.T, activity agentactivity.EventStore, id string) domain.ActivityTurn {
	t.Helper()
	all, err := activity.TurnsSince(t.Context(), lateChat.ID, time.Time{})
	require.NoError(t, err)
	i := slices.IndexFunc(all, func(turn domain.ActivityTurn) bool { return turn.ID == id })
	require.GreaterOrEqual(t, i, 0, "no activity turn %s", id)
	return all[i]
}

func activityTurnByText(t *testing.T, activity agentactivity.EventStore, text string) domain.ActivityTurn {
	t.Helper()
	all, err := activity.TurnsSince(t.Context(), lateChat.ID, time.Time{})
	require.NoError(t, err)
	i := slices.IndexFunc(all, func(turn domain.ActivityTurn) bool { return turn.Text == text })
	require.GreaterOrEqual(t, i, 0, "no activity turn reads %q", text)
	return all[i]
}

// The reply a runner's turn closed on lives as long as the runner: its end
// drops it, so nothing is kept for a runner that is gone.
func TestAbandonMessageForRunner_DropsTheReplyItsLastTurnClosedOn(t *testing.T) {
	turns, _, _ := lateReply(t, "a reply only its hook carried")
	_, held := turns.messages.Answer("c", lateRunner.ID)
	require.True(t, held, "precondition: the hook-only reply is held for a late delta")

	_, err := turns.AbandonMessageForRunner(t.Context(), "c", lateRunner)

	require.NoError(t, err)
	_, held = turns.messages.Answer("c", lateRunner.ID)
	assert.False(t, held)
}

// A reply's late final delta joins its turn only: once the chat has moved to
// another turn, even the same words are its own message.
func TestRecordMessageDelta_ADeltaAfterTheNextTurnStartedIsItsOwnMessage(t *testing.T) {
	activity := &recordingActivity{}
	ledgerStore := newTurnLedger(t)
	turns := &Turns{ledger: ledgerStore, messages: stream.New(), activity: activity, live: newLiveText()}
	chat := domain.Chat{ID: "chat-1"}
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "claude"}
	appendTurnRows(t, ledgerStore, turnRow(t, "runner-1"))
	require.NoError(t, turns.closeAssistantTurn(t.Context(), chat, runner, engineagents.CanonicalEvent{Kind: "turn_stop", Message: "SAID"}))
	ended, err := ledger.NewRow(ledger.KindTurnEnded, "T-runner-1", "", ledger.TurnEnded{Status: "done", RunnerID: "runner-1"})
	require.NoError(t, err)
	appendTurnRows(t, ledgerStore, ended, turnRow(t, "runner-2"))

	require.NoError(t, turns.recordMessageDelta(t.Context(), chat, runner, finalDelta("m2", "SAID AGAIN")))

	assert.Equal(t, []string{"SAID", "SAID AGAIN"}, activity.replies())
}

func TestLiveTextReplaceOverwritesThePreviousCompleteDocument(t *testing.T) {
	live := newLiveText()

	require.Equal(t, "diff v1", live.replace("chat-1", DeltaKindDiff, "turn-1", "diff v1"))
	require.Equal(t, "diff v2", live.replace("chat-1", DeltaKindDiff, "turn-1", "diff v2"))
	require.Equal(t, "diff v2", live.byChat["chat-1"][DeltaKindDiff]["turn-1"].parts[0].String())
}
