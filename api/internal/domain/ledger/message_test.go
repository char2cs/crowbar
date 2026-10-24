package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func text(t *testing.T, turnID, messageID string, index int, final bool, s string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindAssistantText, turnID, ledger.AssistantText{
		MessageID: messageID, Index: index, Final: final, Text: s,
	})
}

func complete(t *testing.T, turnID, messageID, s string) ledger.Row {
	t.Helper()
	return text(t, turnID, messageID, ledger.IndexComplete, false, s)
}

// messages folds rows as the transcript does, one at a time.
func messages(rows []ledger.Row) []ledger.MessageText {
	var f ledger.MessageFold
	for _, r := range rows {
		f.Apply(r)
	}
	return f.Messages()
}

func lateText(t *testing.T, messageID, reply string, index int, final bool, s string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindAssistantText, "T1", ledger.AssistantText{
		MessageID: messageID, Index: index, Final: final, Text: s, Reply: reply,
	})
}

func turnStart(t *testing.T, turnID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindTurnStarted, turnID, ledger.TurnStarted{RunnerID: "run-1", ProviderTurnID: "p-" + turnID})
}

func ended(t *testing.T, turnID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindTurnEnded, turnID, ledger.TurnEnded{Status: "done", RunnerID: "run-1"})
}

func texts(messages []ledger.MessageText) []string {
	out := []string{}
	for _, m := range messages {
		if m.Settled {
			out = append(out, m.Text)
		}
	}
	return out
}

func TestMessages_CompleteFinalStreamWins(t *testing.T) {
	got := messages(sequenced(t,
		text(t, "T1", "m", 0, false, "HEAD "), text(t, "T1", "m", 1, true, "AND TAIL"),
		complete(t, "T1", "m", "TAIL"),
	))

	require.Len(t, got, 1)
	assert.Equal(t, "HEAD AND TAIL", got[0].Text)
	assert.True(t, got[0].Settled)
	assert.Equal(t, int64(2), got[0].SettleSeq, "the final chunk settled it; the shorter report changed nothing")
}

func TestMessages_CompleteRowFillsAGappedStream(t *testing.T) {
	got := messages(sequenced(t,
		text(t, "T1", "m", 0, false, "THE BEGINNING "), text(t, "T1", "m", 2, true, "AND THE END"),
		complete(t, "T1", "m", "THE BEGINNING AND THE MIDDLE AND THE END"),
	))

	require.Len(t, got, 1)
	assert.Equal(t, "THE BEGINNING AND THE MIDDLE AND THE END", got[0].Text)
	assert.Equal(t, int64(3), got[0].SettleSeq)
}

func TestMessages_LateFinalChunkAfterTheCompleteRow(t *testing.T) {
	got := messages(sequenced(t,
		text(t, "T1", "m", 0, false, "SAID "),
		complete(t, "T1", "m", "SAID IN FULL"),
		text(t, "T1", "m", 1, true, "IN FULL"),
	))

	require.Len(t, got, 1)
	assert.Equal(t, "SAID IN FULL", got[0].Text, "the full text, never the tail")
}

func TestMessages_DuplicateChunkIndexesAreIgnored(t *testing.T) {
	got := messages(sequenced(t,
		text(t, "T1", "m", 0, false, "ONE "), text(t, "T1", "m", 0, false, "ONE "),
		text(t, "T1", "m", 1, true, "TWO"),
	))

	require.Len(t, got, 1)
	assert.Equal(t, "ONE TWO", got[0].Text)
}

func TestMessages_AnUnfinishedStreamIsUnsettledUntilItsTurnEnds(t *testing.T) {
	rows := sequenced(t, turnStart(t, "T1"), text(t, "T1", "m", 0, false, "PART"), ended(t, "T1"))

	running := messages(rows[:2])
	require.Len(t, running, 1)
	assert.False(t, running[0].Settled, "a growing message lives on the socket only")

	done := messages(rows)
	require.Len(t, done, 1)
	assert.True(t, done[0].Settled)
	assert.Equal(t, "PART", done[0].Text)
	assert.Equal(t, int64(3), done[0].SettleSeq, "settled by its turn's end")
}

func TestMessages_AGappedStreamSettlesWithEveryChunkItHas(t *testing.T) {
	got := messages(sequenced(t,
		turnStart(t, "T1"), text(t, "T1", "m", 0, false, "A "), text(t, "T1", "m", 2, false, "C"),
		row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}),
	))

	require.Len(t, got, 1)
	assert.Equal(t, "A C", got[0].Text, "a runner's exit keeps what was said")
	assert.True(t, got[0].Settled)
}

func TestMessages_ChunksOutOfOrderFoldIntoOneMessage(t *testing.T) {
	got := messages(sequenced(t,
		turnStart(t, "T1"), complete(t, "T1", ledger.ReplyID("T1"), "ONE TWO THREE"), ended(t, "T1"),
		lateText(t, "m", ledger.ReplyID("T1"), 1, false, "TWO "),
		lateText(t, "m", ledger.ReplyID("T1"), 0, false, "ONE "),
		lateText(t, "m", ledger.ReplyID("T1"), 2, true, "THREE"),
	))

	require.Len(t, got, 1, "arrival order never splits a message")
	assert.Equal(t, ledger.ReplyID("T1"), got[0].Key.MessageID)
	assert.Equal(t, "ONE TWO THREE", got[0].Text)
}

// A turn's end that beat its own final chunk: whichever lands first, the reply
// is one message.
func TestMessages_AReplyIsOneMessageWhicheverOfItsRowsLandsFirst(t *testing.T) {
	for name, rows := range map[string][]ledger.Row{
		"chunk first": {
			turnStart(t, "T1"), text(t, "T1", "m", 0, true, "ONE REPLY"),
			complete(t, "T1", "m", "ONE REPLY"), ended(t, "T1"),
		},
		"end first": {
			turnStart(t, "T1"), complete(t, "T1", ledger.ReplyID("T1"), "ONE REPLY"), ended(t, "T1"),
			lateText(t, "m", ledger.ReplyID("T1"), 0, true, "ONE REPLY"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, []string{"ONE REPLY"}, texts(messages(sequenced(t, rows...))))
		})
	}
}

func TestMessages_ALateExtensionKeepsTheReplysPlaceAndCarriesTheFullText(t *testing.T) {
	got := messages(sequenced(t,
		turnStart(t, "T1"), complete(t, "T1", ledger.ReplyID("T1"), "SAID"), ended(t, "T1"),
		lateText(t, "m", ledger.ReplyID("T1"), 0, true, "SAID IN FULL"),
	))

	require.Len(t, got, 1)
	assert.Equal(t, ledger.ReplyID("T1"), got[0].Key.MessageID)
	assert.Equal(t, "SAID IN FULL", got[0].Text)
	assert.Equal(t, int64(2), got[0].FirstSeq, "the reply keeps its place")
	assert.Equal(t, int64(4), got[0].SettleSeq, "and is listed again once it grew")
}

func TestMessages_ALateMessageWhoseWordsDivergeIsItsOwn(t *testing.T) {
	got := messages(sequenced(t,
		turnStart(t, "T1"), complete(t, "T1", ledger.ReplyID("T1"), "SAID"), ended(t, "T1"),
		lateText(t, "m", ledger.ReplyID("T1"), 0, true, "SOMETHING ELSE"),
	))

	assert.Equal(t, []string{"SAID", "SOMETHING ELSE"}, texts(got))
}

func TestMessages_AnUnfinishedLateStreamJoinsItsReplyAtTheNextEnd(t *testing.T) {
	rows := sequenced(t,
		turnStart(t, "T1"), complete(t, "T1", ledger.ReplyID("T1"), "SAID"), ended(t, "T1"),
		turnStart(t, "T2"),
		lateText(t, "m", ledger.ReplyID("T1"), 0, false, "SAID IN"),
		lateText(t, "m", ledger.ReplyID("T1"), 1, false, " FULL"),
		ended(t, "T2"),
	)

	before := messages(rows[:6])
	assert.Equal(t, []string{"SAID"}, texts(before), "unsettled, it is shown under the reply's id on the socket")

	after := messages(rows)
	require.Len(t, after, 1)
	assert.Equal(t, "T1", after[0].Key.TurnID, "it never touches the next turn")
	assert.Equal(t, "SAID IN FULL", after[0].Text)
}

func TestMessages_ALateFinalChunkAfterTheNextTurnOpenedJoinsItsMessage(t *testing.T) {
	got := messages(sequenced(t,
		turnStart(t, "T1"), text(t, "T1", "m", 0, false, "HALF "), ended(t, "T1"),
		turnStart(t, "T2"),
		text(t, "T1", "m", 1, true, "AND WHOLE"),
	))

	require.Len(t, got, 1, "no second message")
	assert.Equal(t, "HALF AND WHOLE", got[0].Text)
	assert.Equal(t, int64(5), got[0].SettleSeq)
}

func TestState_KeepsWhatARetriedChunkIsRecognisedBy(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		text(t, "T1", "m", 0, false, "A"),
		complete(t, "T1", ledger.ReplyID("T1"), "R"),
		lateText(t, "late", ledger.ReplyID("T1"), 0, false, "R"),
	))

	turn, ok := st.MessageTurn("", "m")
	require.True(t, ok)
	assert.Equal(t, "T1", turn)
	assert.True(t, st.HasChunk("", "m", 0))
	assert.False(t, st.HasChunk("", "m", 1))
	last, ok := st.LastMessage("T1")
	require.True(t, ok)
	assert.Equal(t, "late", last)
	id, reply, ok := st.Reply("T1")
	require.True(t, ok)
	assert.Equal(t, ledger.ReplyID("T1"), id)
	assert.Equal(t, "R", reply)
	assert.Equal(t, ledger.ReplyID("T1"), st.ShownAs("", "late"))
	assert.Equal(t, "m", st.ShownAs("", "m"))
}

func TestCheck_AnAssistantTextNeedsAMessageAndATurn(t *testing.T) {
	require.NoError(t, ledger.Check(ledger.State{}, complete(t, "T1", "m", "x")))
	require.ErrorIs(t, ledger.Check(ledger.State{}, complete(t, "T1", "", "x")), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, complete(t, "", "m", "x")), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, text(t, "T1", "m", -2, false, "x")), ledger.ErrMalformedRow)
	reasoning := row(t, ledger.KindReasoning, "T1", ledger.Reasoning{Text: "thought"})
	require.NoError(t, ledger.Check(ledger.State{}, reasoning))
}
