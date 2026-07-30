package commands_test

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
)

// TestRegression_AbandonTurn_ClosesATurnTheCallerCouldNotSeeYet
//
// THE BUG this rule replaced. A chat whose CLI was taken off it mid-turn was left
// spinning FOREVER: nothing ever closed the turn, and nothing retried. The teardown
// (agent.closeAbandonedTurn) decided for itself whether there was a turn to close, by
// reading domain.AgentChat.Working through GetChat — the READ MODEL, folded by an
// ASYNCHRONOUS projection. The turn was already durable in the event log, but the
// projection had not caught up, so the teardown read a stale "idle" and went home.
//
// The decision therefore moved in HERE, where asynx hands Validate the aggregate folded
// from the event log itself and appends at that same version. This test is that contract:
// an abandon against a chat that IS working must be accepted, no matter what any
// projection currently believes — the caller no longer gets a vote, and cannot lose the
// race it used to lose.
func TestRegression_AbandonTurn_ClosesATurnTheCallerCouldNotSeeYet(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	mid := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(chatAt(now))
	require.True(t, mid.Working, "precondition: the aggregate says a turn is open")

	// The caller passes no opinion of its own — only the chat id. What the read model
	// says about this chat at this instant is not an input to anything.
	abandon := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), Abandoned: true}

	require.NoError(t, abandon.Validate(&mid),
		"an abandon must be accepted whenever the AGGREGATE holds an open turn: this is the "+
			"only thing that will ever close it, and it gets no second chance")
	assert.False(t, abandon.EmitEvent(&mid).Working)
}

// TestRegression_AbandonTurn_WritesNothingToAnIdleChat is the other half, and the reason
// the rule could move into Validate at all.
//
// Making the teardown call AbandonTurn unconditionally is what removes the race — but a
// command that always emits would then write a turn_stopped event, and broadcast a frame,
// every time ANY vendor CLI exits, switches or is stopped, on an event log that is never
// truncated. It would also break the aggregate boundary this package is built on: a
// runner moving off a chat does not write to the chat (see the agent package doc, and
// TestSwitchProvider_Broadcasts_NoChatEvent).
//
// Refusing here keeps both: the caller stays unconditional and the idle chat stays
// untouched. ErrValidation is the ordinary answer, not a fault — asynx appends nothing
// when Validate refuses, so callers must read it as "there was nothing to close".
func TestRegression_AbandonTurn_WritesNothingToAnIdleChat(t *testing.T) {
	now := time.Unix(100, 0).UTC()

	err := commands.StopTurn{ChatID: "c1", Now: now, Abandoned: true}.Validate(chatAt(now))

	require.ErrorIs(t, err, asynxModels.ErrValidation,
		"an abandon on a chat with no turn and no async work must emit no event at all")
}

// TestRegression_AbandonTurn_ClosesAChatSpinningOnAsyncWorkAlone: a chat is Working while
// EITHER a turn is open OR the CLI's last report left async work outstanding, and the
// second case is the one a killed CLI strands — it ends its turn to wait on a background
// task, then dies, and nothing ever restates the level.
//
// So the refusal above must be folded from BOTH inputs, exactly as Working is. A guard
// that only asked "is a turn open?" would refuse here and leave the spinner on forever
// with no turn in sight — the same wedge, one field over.
func TestRegression_AbandonTurn_ClosesAChatSpinningOnAsyncWorkAlone(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	started := commands.StartTurn{ChatID: "c1", Now: now}.EmitEvent(chatAt(now))
	waiting := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 3}.EmitEvent(&started)
	require.True(t, waiting.Working, "precondition: working on async work")
	require.Nil(t, waiting.CurrentTurnStarted, "precondition: with NO turn open")

	abandon := commands.StopTurn{ChatID: "c1", Now: now.Add(2 * time.Second), Abandoned: true}

	require.NoError(t, abandon.Validate(&waiting))
	assert.False(t, abandon.EmitEvent(&waiting).Working)
}

// TestRegression_OrdinaryStopTurn_IsNeverRefused: the refusal is for ABANDON only.
//
// An ordinary turn_stop is the CLI restating the level of async work it left running, and
// a report landing on a chat that currently reads idle is exactly the report that must be
// recorded — it is how a chat goes Working with no turn open. Holding the hook path to the
// abandon's rule would drop it and darken the spinner under live background work.
func TestRegression_OrdinaryStopTurn_IsNeverRefused(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	idle := chatAt(now)
	require.False(t, idle.Working, "precondition: the chat reads idle")

	stop := commands.StopTurn{ChatID: "c1", Now: now.Add(time.Second), AsyncWork: 2}

	require.NoError(t, stop.Validate(idle),
		"a turn_stop hook is a report, not a reconcile: it is never refused for arriving at an idle chat")
	assert.True(t, stop.EmitEvent(idle).Working,
		"and it lights the chat on the level it reports")
}

// TestAbandonTurn_RefusesAChatThatIsGone keeps the no-chat refusal reachable and distinct:
// PurgeChat Forgets the aggregate BEFORE retiring its runners, so the teardown that
// follows targets a chat that no longer exists. It is refused the same way, which is why
// the caller reads ErrValidation as benign in both cases.
func TestAbandonTurn_RefusesAChatThatIsGone(t *testing.T) {
	err := commands.StopTurn{ChatID: "c1", Now: time.Unix(100, 0).UTC(), Abandoned: true}.
		Validate(nil)

	require.ErrorIs(t, err, asynxModels.ErrValidation)
}
