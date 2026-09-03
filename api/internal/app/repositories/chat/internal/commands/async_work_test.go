package commands_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func chatAt(now time.Time) *domain.Chat {
	return &domain.Chat{ID: "c1", WorkspaceID: "w1", CreatedAt: now}
}

// TestStopTurn_WithAsyncWork_KeepsWorking is THE bug, at the fold. claude ends its turn
// the moment it hands work to a background subagent and goes quiet waiting for it — so a
// turn_stop is NOT "the agent is done", and folding Working from the turn alone darkened
// the spinner on a chat whose agent was still working.
func TestStopTurn_WithAsyncWork_KeepsWorking(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	started := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(chatAt(now))

	stopped := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 1}.
		EmitEvent(&started)

	assert.True(t, stopped.Working,
		"a turn that ended with work still running must keep the chat working")
	assert.Nil(t, stopped.CurrentTurnStarted, "the turn itself is genuinely over")
	assert.Equal(t, 1, stopped.AsyncWork)
}

// TestStopTurn_NoAsyncWork_StopsWorking is the ordinary case, and it must be untouched:
// a plain turn with nothing outstanding goes idle exactly as it always did.
func TestStopTurn_NoAsyncWork_StopsWorking(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	started := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(chatAt(now))

	stopped := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 0}.
		EmitEvent(&started)

	assert.False(t, stopped.Working)
	assert.Equal(t, 0, stopped.AsyncWork)
}

// TestStopTurn_RestatedLevelDrains_StopsWorking is the anti-stuck-on guarantee, and the
// reason this is a LEVEL and not a counter. The CLI restates the number on every Stop, so
// the last word always wins and no arithmetic survives between events: 2 outstanding, then
// 0, and the spinner stops. A counter had to receive exactly the right number of retiring
// edges to get here — and measurably never did.
func TestStopTurn_RestatedLevelDrains_StopsWorking(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	c := chatAt(now)

	s1 := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(c)
	waiting := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 2}.EmitEvent(&s1)
	require.True(t, waiting.Working)

	// claude is re-invoked when the work reports back and ends THAT turn clean.
	s2 := commands.StartTurn{ChatID: "c1", Now: now.Add(2 * time.Second)}.EmitEvent(&waiting)
	done := commands.StopTurn{ChatID: "c1", Now: now.Add(3 * time.Second), AsyncWork: 0}.EmitEvent(&s2)

	assert.False(t, done.Working, "a restated level of 0 must stop the spinner")
	assert.Equal(t, 0, done.AsyncWork)
}

// TestStartTurn_SupersedesOutstandingAsyncWork is the self-healing edge that covers the
// one thing the hook surface cannot report AT ALL: an INTERRUPT (ESC) fires no hook —
// not Stop, not Notification, nothing — so a turn the user interrupted is closed by
// nobody, and anything it left standing would stand with it.
//
// A new turn supersedes: the CLI is talking again, and its next turn_stop restates the
// level from scratch, so carrying the old number forward could only preserve a ghost.
func TestStartTurn_SupersedesOutstandingAsyncWork(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	c := chatAt(now)

	s1 := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(c)
	stranded := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 3}.EmitEvent(&s1)
	require.Equal(t, 3, stranded.AsyncWork)

	next := commands.StartTurn{ChatID: "c1", Now: now.Add(2 * time.Second)}.EmitEvent(&stranded)

	assert.Equal(t, 0, next.AsyncWork, "a new turn must not inherit the last turn's claims")
	assert.True(t, next.Working, "it is working because the TURN is open, not because of a ghost")
}

// TestAbandonTurn_ZeroesAsyncWork is what stands between a KILLED CLI and a chat that
// spins forever. Measured: a SIGKILL mid-background-work sends no SessionEnd and no final
// Stop, so the last word is a turn_stop reporting work still running, with nobody left to
// restate it — and this is an event-sourced aggregate, so that word survives a restart.
//
// The reconcile paths (dead runner, displaced runner, boot) come through here.
func TestAbandonTurn_ZeroesAsyncWork(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	c := chatAt(now)

	s1 := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(c)
	waiting := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 3}.EmitEvent(&s1)
	require.True(t, waiting.Working, "precondition: the chat is spinning on announced work")

	abandoned := commands.StopTurn{ChatID: "c1", Now: now.Add(2 * time.Second), Abandoned: true}.
		EmitEvent(&waiting)

	assert.False(t, abandoned.Working, "a dead CLI's chat must not spin forever")
	assert.Equal(t, 0, abandoned.AsyncWork)
	assert.Nil(t, abandoned.CurrentTurnStarted)
}

// TestAbandonTurn_IgnoresReportedLevel: Abandoned is a reconcile, and a reconcile does not
// negotiate with a number the dead CLI left behind. Even handed a level, it zeroes.
func TestAbandonTurn_IgnoresReportedLevel(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	c := chatAt(now)
	s1 := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(c)

	abandoned := commands.StopTurn{
		ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 9, Abandoned: true,
	}.EmitEvent(&s1)

	assert.False(t, abandoned.Working)
	assert.Equal(t, 0, abandoned.AsyncWork)
}

// TestStopTurn_NegativeLevelFloorsToIdle: no CLI can report negative work; it could only
// come from a misconfigured descriptor. Floor it rather than let it sit below zero, where
// it reads as idle anyway but corrupts the next comparison.
func TestStopTurn_NegativeLevelFloorsToIdle(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	c := chatAt(now)
	s1 := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(c)

	stopped := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: -2}.EmitEvent(&s1)

	assert.False(t, stopped.Working)
	assert.Equal(t, 0, stopped.AsyncWork)
}

// TestWorkingIsNeverStrandedOn is the CARDINAL guarantee, stated as a property over every
// sequence a CLI can produce: after ANY run of turn_stops reporting ANY levels, a single
// clean stop (level 0) — or a reconcile — always returns the chat to idle. There is no
// accumulator, so there is no state a hook sequence can wedge.
//
// This is what the previous attempt could not have passed: it counted unpaired edges, so
// a stop it never received left the count standing forever.
func TestWorkingIsNeverStrandedOn(t *testing.T) {
	now := time.Unix(100, 0).UTC()

	for name, levels := range map[string][]int{
		"one background task":      {1},
		"three concurrent":         {3},
		"growing then shrinking":   {3, 4, 1},
		"noisy restatements":       {1, 0, 5, 2, 7},
		"never any async work":     {0, 0, 0},
		"single huge fan-out":      {64},
		"drains then spikes again": {2, 0, 3},
	} {
		t.Run(name, func(t *testing.T) {
			cur := *chatAt(now)
			for i, level := range levels {
				at := now.Add(time.Duration(i) * time.Second)
				started := commands.StartTurn{ChatID: "c1", Now: at}.EmitEvent(&cur)
				cur = commands.StopTurn{ChatID: "c1", Now: at.Add(time.Millisecond), AsyncWork: level}.
					EmitEvent(&started)
			}

			// The CLI's final word: nothing left running.
			last := now.Add(time.Hour)
			started := commands.StartTurn{ChatID: "c1", Now: last}.EmitEvent(&cur)
			settled := commands.StopTurn{ChatID: "c1", Now: last.Add(time.Second), AsyncWork: 0}.
				EmitEvent(&started)

			assert.False(t, settled.Working,
				"no sequence of reported levels may strand the spinner ON")
			assert.Equal(t, 0, settled.AsyncWork)
		})
	}
}
