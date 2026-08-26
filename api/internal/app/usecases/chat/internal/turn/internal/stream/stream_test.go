package stream_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
)

var mnow = time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)

func TestStreams_AssemblesIncrementsInIndexOrder(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 1, true, false, "world", mnow)
	s.Observe("c", "t", "m", 0, true, false, "hello ", mnow)
	message, ok := s.Observe("c", "t", "m", 2, true, true, "!", mnow)

	require.True(t, ok)
	assert.Equal(t, "hello world!", message.Text)
	assert.True(t, message.Final)
	assert.True(t, message.Complete)
}

func TestStreams_IncrementsAreConcatenatedNotReplaced(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 0, true, false, "ALPHA", mnow)
	message, _ := s.Observe("c", "t", "m", 1, true, true, "BETA", mnow)

	assert.Equal(t, "ALPHABETA", message.Text)
}

func TestStreams_TwoMessagesOfOneTurnStaySeparate(t *testing.T) {
	s := stream.New()

	s.Observe("c", "turn-1", "msg-a", 0, true, true, "ALPHA", mnow)
	s.Observe("c", "turn-1", "msg-b", 0, true, true, "OMEGA", mnow)

	open := s.Open("c")
	require.Len(t, open, 2)
	assert.Equal(t, "ALPHA", open[0].Text)
	assert.Equal(t, "OMEGA", open[1].Text)
	assert.Equal(t, "turn-1", open[0].TurnID)
}

func TestStreams_ARedeliveredIncrementIsNotAppendedTwice(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 0, true, false, "once", mnow)
	message, _ := s.Observe("c", "t", "m", 0, true, false, "once", mnow)

	assert.Equal(t, "once", message.Text)
}

func TestStreams_AMissingIncrementIsDetected(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 0, true, false, "start ", mnow)
	message, _ := s.Observe("c", "t", "m", 2, true, true, "end", mnow)

	assert.False(t, message.Complete, "index 1 never arrived and that must be visible")
	assert.Equal(t, "start end", message.Text, "what did arrive is still recorded")
}

func TestStreams_AnIncrementWithNoMessageIDIsDropped(t *testing.T) {
	s := stream.New()

	_, ok := s.Observe("c", "t", "", 0, true, false, "orphan", mnow)

	assert.False(t, ok)
	assert.Empty(t, s.Open("c"))
}

func TestStreams_UnfinishedIsTheInterruptSignal(t *testing.T) {
	s := stream.New()
	s.Observe("c", "t", "done", 0, true, true, "complete", mnow)
	s.Observe("c", "t", "cut", 0, true, false, "half a sen", mnow)

	unfinished := s.Unfinished("c")

	require.Len(t, unfinished, 1)
	assert.Equal(t, "cut", unfinished[0].ID)
	assert.Equal(t, mnow, unfinished[0].LastAt)
}

func TestStreams_TheClockFollowsTheLatestIncrement(t *testing.T) {
	s := stream.New()
	later := mnow.Add(9 * time.Second)

	s.Observe("c", "t", "m", 0, true, false, "one ", mnow)
	message, _ := s.Observe("c", "t", "m", 1, true, false, "two", later)

	assert.Equal(t, later, message.LastAt)
}

func TestStreams_ForgetDropsTheChat(t *testing.T) {
	s := stream.New()
	s.Observe("c", "t", "m", 0, true, true, "text", mnow)

	s.Forget("c")

	assert.Empty(t, s.Open("c"))
	assert.Empty(t, s.Unfinished("c"))
}

func TestStreams_OpenMessagesAreBounded(t *testing.T) {
	s := stream.New()

	for i := range stream.MaxOpenPerChat + 20 {
		s.Observe("c", "t", "m"+strconv.Itoa(i), 0, true, false, "x", mnow)
	}

	assert.Len(t, s.Open("c"), stream.MaxOpenPerChat)
}

// A provider that declares no index: mapping (codex) reports every increment
// at index 0 — this is the exact live bug: two chunks both landing at the
// same slot would fold "P" then "ONG" down to just "ONG" if the index were
// trusted. Unsequenced mode must ignore it and assemble by arrival order.
func TestStreams_UnsequencedIncrementsAssembleByArrivalOrderNotTheGivenIndex(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 0, false, false, "P", mnow)
	message, ok := s.Observe("c", "t", "m", 0, false, true, "ONG", mnow)

	require.True(t, ok)
	assert.Equal(t, "PONG", message.Text)
	assert.True(t, message.Complete)
}

func TestStreams_UnsequencedMessageIsAlwaysCompleteNeverAwaitingAGap(t *testing.T) {
	s := stream.New()

	s.Observe("c", "t", "m", 5, false, false, "a", mnow)
	message, _ := s.Observe("c", "t", "m", 5, false, false, "b", mnow)

	assert.True(t, message.Complete, "arrival order has no future index to wait on")
	assert.Equal(t, "ab", message.Text)
}
