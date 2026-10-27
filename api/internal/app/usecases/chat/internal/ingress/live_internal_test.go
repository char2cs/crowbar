package ingress

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestLive_EveryFrameCarriesTheWholeStream(t *testing.T) {
	l := newLive()
	answer := func(index int, numbered bool, text string) string {
		got, _, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Index: index, Numbered: numbered, Text: text}, at0)
		return got
	}

	assert.Equal(t, "the ", answer(0, false, "the "))
	assert.Equal(t, "the answer", answer(0, false, "answer"), "an unnumbered increment takes the next place")

	l2 := newLive()
	numbered := func(index int, text string) string {
		got, startedAt, _ := l2.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Index: index, Numbered: true, Text: text}, at0)
		assert.Equal(t, at0, startedAt)
		return got
	}
	assert.Equal(t, "b", numbered(1, "b"))
	assert.Equal(t, "ab", numbered(0, "a"), "a numbered one takes its index's place")
}

func TestLive_TheOtherStreamsAppendAndReplace(t *testing.T) {
	l := newLive()
	think := func(index int, text string) string {
		got, _, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "r1", Kind: KindReasoning, Index: index, Text: text}, at0)
		return got
	}
	assert.Equal(t, "hm", think(0, "hm"))
	assert.Equal(t, "hmm", think(0, "m"))
	assert.Equal(t, "hmm\n\nso", think(1, "so"), "its blocks are paragraphs")

	diff := func(text string) string {
		got, _, _ := l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "T1", Kind: KindDiff, Replace: true, Text: text}, at0)
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

// A tool's output is only ever shown: past the window, a frame carries its
// tail behind a marker, so a long stream costs linear bytes, not quadratic.
func TestLive_ALongToolOutputKeepsATailWindow(t *testing.T) {
	l := newLive()
	line := strings.Repeat("é", 511) + "\n"
	var got string
	for i := range 3 * toolOutputWindow / len(line) {
		text := fmt.Sprintf("%04d", i) + line[4:]
		var fd *FeedDelta
		got, _, fd = l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "tool-1", Kind: KindToolOutput, Text: text}, at0)
		require.Equal(t, FeedDelta{ID: "tool-1", TurnID: "T1", Stream: KindToolOutput, N: i + 1, Text: text}, *fd,
			"the feed carries each increment once, past the window too")
	}

	assert.LessOrEqual(t, len(got), toolOutputWindow+64, "a frame never carries more than the window")
	assert.True(t, strings.HasPrefix(got, "[… "), "what is cut is said: %q", got[:32])
	assert.True(t, strings.HasSuffix(got, line[4:]), "the window is the stream's tail")
	assert.True(t, utf8.ValidString(got), "a cut never splits a character")
	for _, b := range l.byChat[testChat] {
		assert.LessOrEqual(t, len(b.join(KindToolOutput)), 2*toolOutputWindow, "what is kept is bounded too")
	}
}

func BenchmarkLive_ObserveALongToolOutput(b *testing.B) {
	line := strings.Repeat("x", 255) + "\n"
	for _, deltas := range []int{4096, 32768} {
		b.Run(fmt.Sprintf("deltas=%d", deltas), func(b *testing.B) {
			for range b.N {
				l := newLive()
				for range deltas {
					l.observe(testChat, "run-1", LiveDelta{TurnID: "T1", ID: "tool-1", Kind: KindToolOutput, Text: line}, at0)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*deltas), "ns/delta")
		})
	}
}

// fed is the feed delta observing d gives, nil when it gives none.
func fed(l *live, runnerID string, d LiveDelta) *FeedDelta {
	_, _, fd := l.observe(testChat, runnerID, d, at0)
	return fd
}

func TestLive_EachDeltaCarriesOnlyTheNewSuffix(t *testing.T) {
	l := newLive()
	var got []FeedDelta
	for _, text := range []string{"a", "b", "c"} {
		fd := fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Text: text})
		require.NotNil(t, fd)
		got = append(got, *fd)
	}
	think := fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "r1", Kind: KindReasoning, Text: "hm"})
	paragraph := fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "r1", Kind: KindReasoning, Index: 1, Text: "so"})

	assert.Equal(t, []FeedDelta{
		{ID: "m1", TurnID: "T1", Stream: StreamAnswer, N: 1, Text: "a"},
		{ID: "m1", TurnID: "T1", Stream: StreamAnswer, N: 2, Text: "b"},
		{ID: "m1", TurnID: "T1", Stream: StreamAnswer, N: 3, Text: "c"},
	}, got, "the stream so far is a, ab, abc: each frame is what it added")
	assert.Equal(t, FeedDelta{ID: "r1", TurnID: "T1", Stream: KindReasoning, N: 1, Text: "hm"}, *think)
	assert.Equal(t, FeedDelta{ID: "r1", TurnID: "T1", Stream: KindReasoning, N: 2, Text: "\n\nso"}, *paragraph,
		"numbered per stream, and a new block is its separator too")
	assert.Nil(t, fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "n", Numbered: true, Text: "x"}),
		"a numbered increment is a row: the row frame carries it")
}

func TestLive_ARewriteThatDoesNotExtendIsAReplace(t *testing.T) {
	l := newLive()
	diff := func(text string) *FeedDelta {
		return fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "T1", Kind: KindDiff, Replace: true, Text: text})
	}
	for _, text := range []string{"a", "ab", "abc"} {
		require.False(t, diff(text).Replace, "a restatement that extends the text is its suffix")
	}

	assert.Equal(t, &FeedDelta{ID: "T1", TurnID: "T1", Stream: KindDiff, N: 4, Text: "xy", Replace: true}, diff("xy"))
	assert.Nil(t, diff("xy"), "a restatement that changes nothing is no frame")
	assert.Equal(t, 5, diff("xyz").N)
}

func TestLive_SnapshotHasOneReplacePerLiveStream(t *testing.T) {
	l := newLive()
	fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Text: "one"})
	fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "m1", Text: " more"})
	fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "tool-1", Kind: KindToolOutput, Text: "$ ls"})
	fed(l, "run-2", LiveDelta{TurnID: "T1", Scope: "agent-1", ID: "m2", Text: "two"})
	fed(l, "run-1", LiveDelta{TurnID: "T1", ID: "n", Numbered: true, Text: "a row holds this"})
	l.observe("chat-2", "run-1", LiveDelta{TurnID: "T1", ID: "m3", Text: "elsewhere"}, at0)

	assert.Equal(t, []FeedDelta{
		{ID: "m1", TurnID: "T1", Stream: StreamAnswer, N: 2, Text: "one more", Replace: true},
		{ID: "tool-1", TurnID: "T1", Stream: KindToolOutput, N: 1, Text: "$ ls", Replace: true},
		{ID: "m2", TurnID: "T1", Scope: "agent-1", Stream: StreamAnswer, N: 1, Text: "two", Replace: true},
	}, l.snapshot(testChat), "in the order they began")
	assert.Empty(t, l.snapshot("chat-3"))
}

func TestLive_ACompleteRowDropsItsStreamFromTheSnapshot(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	for _, ev := range []engineagents.CanonicalEvent{
		delta(engineagents.HookMessageDelta, "m1", false, false, "hel"),
		delta(engineagents.HookMessageDelta, "m2", false, false, "still going"),
	} {
		_, _, err = f.dispatch(t, ev)
		require.NoError(t, err)
	}
	require.Len(t, f.r.Snapshot(testChat), 2)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind:     engineagents.HookMessageComplete,
		Complete: &engineagents.MessageComplete{MessageID: "m1", Text: "hello"},
	})
	require.NoError(t, err)

	snap := f.r.Snapshot(testChat)
	require.Len(t, snap, 1)
	assert.Equal(t, "m2", snap[0].ID, "the row holds m1 now")
	assert.Equal(t, []FeedDelta{
		{ID: "m1", TurnID: snap[0].TurnID, Stream: StreamAnswer, N: 1, Text: "hel"},
		{ID: "m2", TurnID: snap[0].TurnID, Stream: StreamAnswer, N: 1, Text: "still going"},
	}, f.feed.deltas, "each unnumbered delta is fed once its rows commit")
}
