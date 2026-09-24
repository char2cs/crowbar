package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Invariant A1 (spec §7-A), the interruption half: after StopChat returns,
// exactly one "stopped" interruption exists if a turn was open, and none if
// the chat was idle. These run against the REAL turn ingress, so a teardown
// path that completes the in-flight turn before the interruption is recorded
// (displace does exactly that) cannot hide behind a spy.

func stoppedInterruptions(t *testing.T, f testFixture, chatID string) int {
	t.Helper()
	f.wait()
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	n := 0
	for _, in := range ints {
		if in.Kind == engineagents.InterruptStopped {
			n++
		}
	}
	return n
}

func TestInvariantA1_StopMidTurnOnTheRetirePathRecordsExactlyOneStop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID),
		"a Stop that retires a mid-turn CLI must leave exactly one Interrupted divider")
}

func TestInvariantA1_StopOnAnIdleChatRecordsNoStop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "done")
	require.False(t, f.chat(t, chatID).Working, "precondition: the chat is idle")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Zero(t, stoppedInterruptions(t, f, chatID), "closing an idle chat interrupts nothing")
}

func TestInvariantA1_ASecondStopRecordsNothingMore(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID))
}

func TestInvariantA1_AForcedSwitchRecordsExactlyOneStop(t *testing.T) {
	f := newFixture(t)
	agentusecase.SetSwitchAwaitTimeout(f.usecase.RunnerUsecase, 10*time.Millisecond)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)

	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID),
		"forcing the outgoing turn is a Stop, and must say so exactly once")
}

// Invariant A2: StopChat completes within a bound while a switch is in flight.
// The switch below parks on a turn that never ends, with an hour's grace
// period; before the gate became preemptible Stop queued behind it for the
// whole of that hour.
func TestInvariantA2_StopPreemptsASwitchParkedOnTheTurn(t *testing.T) {
	f := newFixture(t)
	agentusecase.SetSwitchAwaitTimeout(f.usecase.RunnerUsecase, time.Hour)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	parked := parkedOnTurn(t)
	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()
	select {
	case <-parked:
	case r := <-done:
		t.Fatalf("the switch returned without parking: %+v", r)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- f.usecase.StopChat(f.ctx, chatID) }()

	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("StopChat queued behind a parked switch instead of preempting it")
	}

	got := <-done
	require.ErrorIs(t, got.err, agentusecase.ErrStopped, "the preempted switch reports that Stop won")
	assert.Empty(t, got.runnerID)

	_, err := f.liveRunnerFor(t, chatID)
	assert.Error(t, err, "after Stop the chat is dormant: the preempted switch spawned nothing")
	assert.Equal(t, 1, stoppedInterruptions(t, f, chatID))
}
