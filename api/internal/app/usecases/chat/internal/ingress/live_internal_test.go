package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestLive_EveryFrameCarriesTheWholeStream(t *testing.T) {
	l := newLive()
	answer := func(index int, numbered bool, text string) string {
		got, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Index: index, Numbered: numbered, Text: text}, at0)
		return got
	}

	assert.Equal(t, "the ", answer(0, false, "the "))
	assert.Equal(t, "the answer", answer(0, false, "answer"), "an unnumbered increment takes the next place")

	l2 := newLive()
	numbered := func(index int, text string) string {
		got, startedAt := l2.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Index: index, Numbered: true, Text: text}, at0)
		assert.Equal(t, at0, startedAt)
		return got
	}
	assert.Equal(t, "b", numbered(1, "b"))
	assert.Equal(t, "ab", numbered(0, "a"), "a numbered one takes its index's place")
}

func TestLive_TheOtherStreamsAppendAndReplace(t *testing.T) {
	l := newLive()
	think := func(index int, text string) string {
		got, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "r1", Kind: KindReasoning, Index: index, Text: text}, at0)
		return got
	}
	assert.Equal(t, "hm", think(0, "hm"))
	assert.Equal(t, "hmm", think(0, "m"))
	assert.Equal(t, "hmm\n\nso", think(1, "so"), "its blocks are paragraphs")

	diff := func(text string) string {
		got, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "T1", Kind: KindDiff, Replace: true, Text: text}, at0)
		return got
	}
	diff("+a")
	assert.Equal(t, "+b", diff("+b"), "a whole restatement replaces the stream")
}

func TestLive_UnrecordedIsWhatNoRowHolds(t *testing.T) {
	l := newLive()
	observe := func(runnerID string, d LiveDelta) { l.observe(testChat, runnerID, d, at0) }
	observe("run-1", LiveDelta{TurnID: "T1", ID: "b", Text: "second"})
	observe("run-1", LiveDelta{TurnID: "T1", ID: "a", Text: "first"})
	observe("run-1", LiveDelta{TurnID: "T1", ID: "n", Numbered: true, Text: "a row holds this"})
	observe("run-1", LiveDelta{TurnID: "T2", ID: "c", Text: "another turn"})
	observe("run-1", LiveDelta{TurnID: "T1", ID: "e", Text: ""})
	observe("run-2", LiveDelta{TurnID: "T1", ID: "d", Text: "another runner"})
	observe("run-1", LiveDelta{TurnID: "T1", ID: "r", Kind: KindReasoning, Text: "thought"})

	assert.Equal(t, []liveText{
		{TurnID: "T1", ID: "b", Text: "second"}, {TurnID: "T1", ID: "a", Text: "first"},
	}, l.unrecorded(testChat, "run-1", "T1", KindAnswer), "in the order they were said")
	assert.Len(t, l.unrecorded(testChat, "run-1", "", KindAnswer), 3)
	assert.Equal(t, []liveText{{TurnID: "T1", ID: "r", Text: "thought"}}, l.unrecorded(testChat, "run-1", "", KindReasoning))
}

func TestLive_ForgetDropsWhatEnded(t *testing.T) {
	l := newLive()
	l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "a", Text: "one"}, at0)
	l.observe(testChat, "run-1", LiveDelta{TurnID: "T2", ID: "b", Text: "two"}, at0)
	l.observe("chat-2", "run-1", LiveDelta{TurnID: "T1", ID: "c", Text: "elsewhere"}, at0)

	l.forget(testChat, "run-1", "T1")
	assert.Equal(t, []liveText{{TurnID: "T2", ID: "b", Text: "two"}}, l.unrecorded(testChat, "run-1", "", KindAnswer))

	l.forget(testChat, "run-1", "")
	assert.Empty(t, l.unrecorded(testChat, "run-1", "", KindAnswer))
	assert.NotContains(t, l.byChat, testChat)

	l.forgetChat("chat-2")
	assert.Empty(t, l.byChat)
}

func TestRecorder_AFrameIsShownOnlyOnceItsRowsCommit(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", true, false, "never recorded"))

	require.Error(t, err)
	assert.Empty(t, f.feed.frames)
}

// A late stream shown as the reply its turn closed on never shows less of it.
func TestRecorder_AJoinedStreamsFrameNeverShowsLessThanItsReply(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	for _, ev := range []engineagents.CanonicalEvent{
		{Kind: engineagents.HookUserPrompt, Message: "go"},
		{Kind: engineagents.HookTurnStop, Message: "Hello world"},
	} {
		_, _, err := f.dispatch(t, ev)
		require.NoError(t, err)
	}
	turn, _ := f.state(t).LastTurn()
	chunk := func(index int, text string) {
		ev := delta(engineagents.HookMessageDelta, "m1", true, false, text)
		ev.Delta.Index = index
		_, _, err := f.dispatch(t, ev)
		require.NoError(t, err)
	}

	chunk(0, "Hello")
	chunk(1, " world, and more")

	require.Len(t, f.feed.frames, 2)
	assert.Equal(t, frame{ledger.ReplyID(turn.ID), "Hello world", "", f.feed.frames[0].startedAt}, f.feed.frames[0])
	assert.Equal(t, ledger.ReplyID(turn.ID), f.feed.frames[1].messageID, "every chunk is shown as the reply")
	assert.Equal(t, "Hello world, and more", f.feed.frames[1].text)
}

// Every frame carries the stream so far; what its turn's end recorded is
// dropped with it.
func TestRecorder_StreamedTextIsShownWholeUntilItsTurnEnds(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	for _, ev := range []engineagents.CanonicalEvent{
		delta(engineagents.HookMessageDelta, "m1", false, false, "the "),
		delta(engineagents.HookMessageDelta, "m1", false, false, "answer"),
		delta(engineagents.HookReasoningDelta, "r1", false, false, "hm"),
	} {
		_, _, err = f.dispatch(t, ev)
		require.NoError(t, err)
	}
	require.Len(t, f.feed.frames, 3)
	assert.Equal(t, "the answer", f.feed.frames[1].text)
	assert.False(t, f.feed.frames[1].startedAt.IsZero(), "an answer says when it began")
	assert.True(t, f.feed.frames[2].startedAt.IsZero())
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Len(t, exit, 1, "what no row holds is the exit's to record")

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop})
	require.NoError(t, err)

	exit, err = f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit, "the end recorded it")
	assert.Equal(t, []string{"the answer"}, said(f.rows(t)))
}

func TestRecorder_AnIdleThatEndsTheTurnKeepsItsStreamedText(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", false, false, "said"))
	require.NoError(t, err)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookIdle})

	require.NoError(t, err)
	assert.Equal(t, []string{"said"}, said(f.rows(t)))
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit, "the end recorded it")
}

func TestRecorder_RetireAndForgetDropTheStreams(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.dispatch(t, delta(engineagents.HookMessageDelta, "m1", false, false, "x"))
	require.NoError(t, err)

	f.r.Retire(testChat, "run-1")
	exit, err := f.r.ExitRows(f.state(t), testChat, "run-1")
	require.NoError(t, err)
	assert.Empty(t, exit)

	f.r.ArmManualCompaction(testChat)
	f.r.ForgetChat(testChat)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookCompactPre})
	require.NoError(t, err)
	rows := f.rows(t)
	assert.Empty(t, bodyOf[ledger.Marker](t, rows[len(rows)-1]).Detail, "a forgotten chat's request is gone")
}
