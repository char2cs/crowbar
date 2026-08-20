package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The assembler is reached directly here because it IS a state machine over
// out-of-order, at-least-once, possibly-incomplete input, and the states that
// matter most are the ones a healthy provider never produces.

var mnow = time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)

func TestMessageStreams_AssemblesIncrementsInIndexOrder(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 1, false, "world", mnow)
	s.observe("c", "t", "m", 0, false, "hello ", mnow)
	buffer, ok := s.observe("c", "t", "m", 2, true, "!", mnow)

	require.True(t, ok)
	assert.Equal(t, "hello world!", buffer.Text())
	assert.True(t, buffer.Final)
	assert.True(t, buffer.Complete())
}

// Increments are ADDITIONS, not cumulative snapshots. Treating them as snapshots
// would record only the last chunk of every message.
func TestMessageStreams_IncrementsAreConcatenatedNotReplaced(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "ALPHA", mnow)
	buffer, _ := s.observe("c", "t", "m", 1, true, "BETA", mnow)

	assert.Equal(t, "ALPHABETA", buffer.Text())
}

// TestMessageStreams_TwoMessagesOfOneTurnStaySeparate is the defect the whole
// mechanism exists for. Grouping by turn alone would concatenate two things the
// agent said into one thing it never said.
func TestMessageStreams_TwoMessagesOfOneTurnStaySeparate(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "turn-1", "msg-a", 0, true, "ALPHA", mnow)
	s.observe("c", "turn-1", "msg-b", 0, true, "OMEGA", mnow)

	open := s.openMessages("c")
	require.Len(t, open, 2)
	assert.Equal(t, "ALPHA", open[0].Text())
	assert.Equal(t, "OMEGA", open[1].Text())
	assert.Equal(t, "turn-1", open[0].TurnID)
}

// Hook delivery is at-least-once — the relay retries — so the same increment can
// arrive twice. The index is what makes a repeat identifiable as one; appending
// blindly would duplicate text inside a message.
func TestMessageStreams_ARedeliveredIncrementIsNotAppendedTwice(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "once", mnow)
	buffer, _ := s.observe("c", "t", "m", 0, false, "once", mnow)

	assert.Equal(t, "once", buffer.Text())
}

// TestMessageStreams_AMissingIncrementIsDetected.
//
// Hook delivery has no acknowledgement, so a chunk that never arrives is
// otherwise a message quietly recorded short — the one failure mode with no
// symptom. A gap in the index is the only signal there is.
func TestMessageStreams_AMissingIncrementIsDetected(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "start ", mnow)
	buffer, _ := s.observe("c", "t", "m", 2, true, "end", mnow)

	assert.False(t, buffer.Complete(), "index 1 never arrived and that must be visible")
	assert.Equal(t, "start end", buffer.Text(), "what did arrive is still recorded")
}

// An increment with no message id cannot be grouped, and appending it to whatever
// came last would attribute text to a message that did not contain it.
func TestMessageStreams_AnIncrementWithNoMessageIDIsDropped(t *testing.T) {
	s := newMessageStreams()

	_, ok := s.observe("c", "t", "", 0, false, "orphan", mnow)

	assert.False(t, ok)
	assert.Empty(t, s.openMessages("c"))
}

// TestMessageStreams_UnfinishedIsTheInterruptSignal. A finished message ended the
// way it should; an unfinished one is the only evidence a human interrupt leaves,
// because the provider fires no hook for one.
func TestMessageStreams_UnfinishedIsTheInterruptSignal(t *testing.T) {
	s := newMessageStreams()
	s.observe("c", "t", "done", 0, true, "complete", mnow)
	s.observe("c", "t", "cut", 0, false, "half a sen", mnow)

	unfinished := s.unfinished("c")

	require.Len(t, unfinished, 1)
	assert.Equal(t, "cut", unfinished[0].ID)
	assert.Equal(t, mnow, unfinished[0].LastAt)
}

// The clock a detector reads must track the LAST increment, not the first: a
// message still growing has not been abandoned however long ago it started.
func TestMessageStreams_TheClockFollowsTheLatestIncrement(t *testing.T) {
	s := newMessageStreams()
	later := mnow.Add(9 * time.Second)

	s.observe("c", "t", "m", 0, false, "one ", mnow)
	buffer, _ := s.observe("c", "t", "m", 1, false, "two", later)

	assert.Equal(t, later, buffer.LastAt)
}

func TestMessageStreams_ForgetDropsTheChat(t *testing.T) {
	s := newMessageStreams()
	s.observe("c", "t", "m", 0, true, "text", mnow)

	s.forget("c")

	assert.Empty(t, s.openMessages("c"))
	assert.Empty(t, s.unfinished("c"))
}

// A provider that never sends a terminating increment must not be able to grow
// this without limit.
func TestMessageStreams_OpenMessagesAreBounded(t *testing.T) {
	s := newMessageStreams()

	for i := range maxOpenMessagesPerChat + 20 {
		s.observe("c", "t", "m"+itoa(int64(i)), 0, false, "x", mnow)
	}

	assert.Len(t, s.openMessages("c"), maxOpenMessagesPerChat)
}
