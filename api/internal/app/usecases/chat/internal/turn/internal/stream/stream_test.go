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

	s.Observe("c", "r", "t", "m", 1, true, false, "world", mnow)
	s.Observe("c", "r", "t", "m", 0, true, false, "hello ", mnow)
	message, ok := s.Observe("c", "r", "t", "m", 2, true, true, "!", mnow)

	require.True(t, ok)
	assert.Equal(t, "hello world!", message.Text)
	assert.True(t, message.Final)
	assert.True(t, message.Complete)
}

func TestStreams_IncrementsAreConcatenatedNotReplaced(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "t", "m", 0, true, false, "ALPHA", mnow)
	message, _ := s.Observe("c", "r", "t", "m", 1, true, true, "BETA", mnow)

	assert.Equal(t, "ALPHABETA", message.Text)
}

func TestStreams_TwoMessagesOfOneTurnStaySeparate(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "turn-1", "msg-a", 0, true, true, "ALPHA", mnow)
	s.Observe("c", "r", "turn-1", "msg-b", 0, true, true, "OMEGA", mnow)

	open := s.Open("c", "r")
	require.Len(t, open, 2)
	assert.Equal(t, "ALPHA", open[0].Text)
	assert.Equal(t, "OMEGA", open[1].Text)
	assert.Equal(t, "turn-1", open[0].TurnID)
}

func TestStreams_ARedeliveredIncrementIsNotAppendedTwice(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "t", "m", 0, true, false, "once", mnow)
	message, _ := s.Observe("c", "r", "t", "m", 0, true, false, "once", mnow)

	assert.Equal(t, "once", message.Text)
}

func TestStreams_AMissingIncrementIsDetected(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "t", "m", 0, true, false, "start ", mnow)
	message, _ := s.Observe("c", "r", "t", "m", 2, true, true, "end", mnow)

	assert.False(t, message.Complete, "index 1 never arrived and that must be visible")
	assert.Equal(t, "start end", message.Text, "what did arrive is still recorded")
}

func TestStreams_AnIncrementWithNoMessageIDIsDropped(t *testing.T) {
	s := stream.New()

	_, ok := s.Observe("c", "r", "t", "", 0, true, false, "orphan", mnow)

	assert.False(t, ok)
	assert.Empty(t, s.Open("c", "r"))
}

func TestStreams_UnfinishedIsTheInterruptSignal(t *testing.T) {
	s := stream.New()
	s.Observe("c", "r", "t", "done", 0, true, true, "complete", mnow)
	s.Observe("c", "r", "t", "cut", 0, true, false, "half a sen", mnow)

	unfinished := s.Unfinished("c", "r")

	require.Len(t, unfinished, 1)
	assert.Equal(t, "cut", unfinished[0].ID)
	assert.Equal(t, mnow, unfinished[0].LastAt)
}

func TestStreams_TheClockFollowsTheLatestIncrement(t *testing.T) {
	s := stream.New()
	later := mnow.Add(9 * time.Second)

	s.Observe("c", "r", "t", "m", 0, true, false, "one ", mnow)
	message, _ := s.Observe("c", "r", "t", "m", 1, true, false, "two", later)

	assert.Equal(t, later, message.LastAt)
}

func TestStreams_ForgetDropsTheChat(t *testing.T) {
	s := stream.New()
	s.Observe("c", "r", "t", "m", 0, true, true, "text", mnow)

	s.Forget("c", "r")

	assert.Empty(t, s.Open("c", "r"))
	assert.Empty(t, s.Unfinished("c", "r"))
}

func TestStreams_OpenMessagesAreBounded(t *testing.T) {
	s := stream.New()

	for i := range stream.MaxOpenPerChat + 20 {
		s.Observe("c", "r", "t", "m"+strconv.Itoa(i), 0, true, false, "x", mnow)
	}

	assert.Len(t, s.Open("c", "r"), stream.MaxOpenPerChat)
}

// A provider that declares no index: mapping (codex) reports every increment
// at index 0 — this is the exact live bug: two chunks both landing at the
// same slot would fold "P" then "ONG" down to just "ONG" if the index were
// trusted. Unsequenced mode must ignore it and assemble by arrival order.
func TestStreams_UnsequencedIncrementsAssembleByArrivalOrderNotTheGivenIndex(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "t", "m", 0, false, false, "P", mnow)
	message, ok := s.Observe("c", "r", "t", "m", 0, false, true, "ONG", mnow)

	require.True(t, ok)
	assert.Equal(t, "PONG", message.Text)
	assert.True(t, message.Complete)
}

func TestStreams_UnsequencedMessageIsAlwaysCompleteNeverAwaitingAGap(t *testing.T) {
	s := stream.New()

	s.Observe("c", "r", "t", "m", 5, false, false, "a", mnow)
	message, _ := s.Observe("c", "r", "t", "m", 5, false, false, "b", mnow)

	assert.True(t, message.Complete, "arrival order has no future index to wait on")
	assert.Equal(t, "ab", message.Text)
}

// ── Runner isolation ────────────────────────────────────────────────────
// Regression: Open/Unfinished/IndexOf/Forget used to be scoped only by
// chatID. Two runners can genuinely have messages open on the SAME chat at
// once — "interrupted" is a graceful stop request, not a kill, so a runner
// asked to stop can still be mid-message when a DIFFERENT runner (a
// provider switch) opens and closes its own turn on the same chat. An
// unfiltered Open handed the closing runner the OTHER runner's still-open
// text as if it were its own — recorded under the wrong provider — and an
// unfiltered Forget then deleted it before the real owner ever got to close
// it correctly.

func TestStreams_OpenIsScopedToOneRunner_NotTheWholeChat(t *testing.T) {
	s := stream.New()
	s.Observe("c", "claude-runner", "t1", "claude-msg", 0, true, false, "claude reply", mnow)
	s.Observe("c", "codex-runner", "t2", "codex-msg", 0, true, true, "codex reply", mnow)

	claudeOpen := s.Open("c", "claude-runner")
	require.Len(t, claudeOpen, 1)
	assert.Equal(t, "claude-msg", claudeOpen[0].ID)

	codexOpen := s.Open("c", "codex-runner")
	require.Len(t, codexOpen, 1)
	assert.Equal(t, "codex-msg", codexOpen[0].ID)
}

func TestStreams_ForgetOnlyDropsTheCallingRunnersMessages(t *testing.T) {
	s := stream.New()
	s.Observe("c", "claude-runner", "t1", "claude-msg", 0, true, false, "still going", mnow)
	s.Observe("c", "codex-runner", "t2", "codex-msg", 0, true, true, "done", mnow)

	// Codex closes and forgets its OWN turn.
	s.Forget("c", "codex-runner")

	assert.Empty(t, s.Open("c", "codex-runner"), "codex's own entry is gone")
	claudeOpen := s.Open("c", "claude-runner")
	require.Len(t, claudeOpen, 1, "claude's still-open message survives a DIFFERENT runner's Forget")
	assert.Equal(t, "still going", claudeOpen[0].Text)
}

func TestStreams_IndexOfIsScopedToOneRunner(t *testing.T) {
	s := stream.New()
	s.Observe("c", "claude-runner", "t1", "claude-msg", 0, true, false, "x", mnow)
	// Codex's own two-item turn, interleaved in arrival order with Claude's.
	s.Observe("c", "codex-runner", "t2", "codex-msg-1", 0, true, true, "first", mnow)
	s.Observe("c", "codex-runner", "t2", "codex-msg-2", 0, true, true, "second", mnow)

	// Codex's items are 0 and 1 within CODEX's own turn — Claude's unrelated,
	// still-open message must not consume a slot in Codex's numbering.
	assert.Equal(t, 0, s.IndexOf("c", "codex-runner", "codex-msg-1"))
	assert.Equal(t, 1, s.IndexOf("c", "codex-runner", "codex-msg-2"))
}

func TestStreams_UnfinishedAcrossRunnersSeesEveryRunnersOwnUnfinishedMessage(t *testing.T) {
	s := stream.New()
	s.Observe("c", "claude-runner", "t1", "claude-msg", 0, true, false, "still going", mnow)
	s.Observe("c", "codex-runner", "t2", "codex-msg", 0, true, false, "also going", mnow)

	unfinished := s.UnfinishedAcrossRunners("c")

	require.Len(t, unfinished, 2, "sees both runners' unfinished messages, unlike Unfinished(chatID, oneRunnerID)")
}

func TestStreams_AwaitOpen_ReturnsImmediatelyWhenAlreadyOpen(t *testing.T) {
	s := stream.New()
	s.Observe("c", "r", "t", "m", 0, true, false, "already streaming", mnow)

	// A timeout long enough to fail the test if AwaitOpen actually waited on
	// it instead of returning the already-open message straight away.
	open := s.AwaitOpen("c", "r", time.Minute)

	require.Len(t, open, 1)
	assert.Equal(t, "already streaming", open[0].Text)
}

func TestStreams_AwaitOpen_GivesUpAfterTheTimeoutWhenNothingArrives(t *testing.T) {
	s := stream.New()

	open := s.AwaitOpen("c", "r", 5*time.Millisecond)

	assert.Empty(t, open, "nothing ever streamed for this runner, so the wait must still end and report empty")
}
