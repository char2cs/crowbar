package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mnow = time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)

func TestMessageStreams_AssemblesIncrementsInIndexOrder(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 1, false, "world", mnow)
	s.observe("c", "t", "m", 0, false, "hello ", mnow)
	message, ok := s.observe("c", "t", "m", 2, true, "!", mnow)

	require.True(t, ok)
	assert.Equal(t, "hello world!", message.Text)
	assert.True(t, message.Final)
	assert.True(t, message.Complete)
}

func TestMessageStreams_IncrementsAreConcatenatedNotReplaced(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "ALPHA", mnow)
	message, _ := s.observe("c", "t", "m", 1, true, "BETA", mnow)

	assert.Equal(t, "ALPHABETA", message.Text)
}

func TestMessageStreams_TwoMessagesOfOneTurnStaySeparate(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "turn-1", "msg-a", 0, true, "ALPHA", mnow)
	s.observe("c", "turn-1", "msg-b", 0, true, "OMEGA", mnow)

	open := s.openMessages("c")
	require.Len(t, open, 2)
	assert.Equal(t, "ALPHA", open[0].Text)
	assert.Equal(t, "OMEGA", open[1].Text)
	assert.Equal(t, "turn-1", open[0].TurnID)
}

func TestMessageStreams_ARedeliveredIncrementIsNotAppendedTwice(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "once", mnow)
	message, _ := s.observe("c", "t", "m", 0, false, "once", mnow)

	assert.Equal(t, "once", message.Text)
}

func TestMessageStreams_AMissingIncrementIsDetected(t *testing.T) {
	s := newMessageStreams()

	s.observe("c", "t", "m", 0, false, "start ", mnow)
	message, _ := s.observe("c", "t", "m", 2, true, "end", mnow)

	assert.False(t, message.Complete, "index 1 never arrived and that must be visible")
	assert.Equal(t, "start end", message.Text, "what did arrive is still recorded")
}

func TestMessageStreams_AnIncrementWithNoMessageIDIsDropped(t *testing.T) {
	s := newMessageStreams()

	_, ok := s.observe("c", "t", "", 0, false, "orphan", mnow)

	assert.False(t, ok)
	assert.Empty(t, s.openMessages("c"))
}

func TestMessageStreams_UnfinishedIsTheInterruptSignal(t *testing.T) {
	s := newMessageStreams()
	s.observe("c", "t", "done", 0, true, "complete", mnow)
	s.observe("c", "t", "cut", 0, false, "half a sen", mnow)

	unfinished := s.unfinished("c")

	require.Len(t, unfinished, 1)
	assert.Equal(t, "cut", unfinished[0].ID)
	assert.Equal(t, mnow, unfinished[0].LastAt)
}

func TestMessageStreams_TheClockFollowsTheLatestIncrement(t *testing.T) {
	s := newMessageStreams()
	later := mnow.Add(9 * time.Second)

	s.observe("c", "t", "m", 0, false, "one ", mnow)
	message, _ := s.observe("c", "t", "m", 1, false, "two", later)

	assert.Equal(t, later, message.LastAt)
}

func TestMessageStreams_ForgetDropsTheChat(t *testing.T) {
	s := newMessageStreams()
	s.observe("c", "t", "m", 0, true, "text", mnow)

	s.forget("c")

	assert.Empty(t, s.openMessages("c"))
	assert.Empty(t, s.unfinished("c"))
}

func TestMessageStreams_OpenMessagesAreBounded(t *testing.T) {
	s := newMessageStreams()

	for i := range maxOpenMessagesPerChat + 20 {
		s.observe("c", "t", "m"+itoa(int64(i)), 0, false, "x", mnow)
	}

	assert.Len(t, s.openMessages("c"), maxOpenMessagesPerChat)
}
