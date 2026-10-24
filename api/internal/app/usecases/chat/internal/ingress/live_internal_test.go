package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
