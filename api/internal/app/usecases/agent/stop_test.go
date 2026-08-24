package agent_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// TestStopChat_TerminatesTheCLI_LeavesChatDormantAndResumable is the headline: closing
// a chat tab STOPS the vendor CLI but must leave the chat exactly where a later reopen
// can revive the REAL conversation. It drives the whole life-cycle a close touches —
// terminate the live runner, drop to dormant, clear a mid-turn spinner, keep the chat
// and its bound conversation — and then proves resumability end-to-end via ResumeChat.
func TestStopChat_TerminatesTheCLI_LeavesChatDormantAndResumable(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	require.NotEmpty(t, oldTerm)

	// A real, resumable conversation: the CLI announced its native session and took a
	// turn (a session id is not a conversation until something is written to it).
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said something")

	// And it is mid-answer when the user closes the tab — the state whose spinner the
	// close must clear.
	prompt(t, f, runnerID, "claude", "and one more thing")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	// (1) the vendor CLI was gracefully terminated.
	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "close must gracefully terminate the live CLI")

	// (2) the chat is DORMANT — no runner points at it any more.
	_, err := f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "after close the chat has no live runner")

	// (3) the chat still EXISTS, and its Working spinner was cleared (the aborted turn).
	stopped := f.chat(t, chatID)
	assert.False(t, stopped.Working, "closing mid-turn must clear the spinner, not leave it spinning forever")

	// (4) its bound conversation is retained — this is what makes it resumable.
	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, convs, 1)
	assert.Equal(t, "sid-claude-native", convs[0].SessionID, "a close must keep the conversation it can be resumed into")

	// The outgoing runner stays alive until its PTY actually dies — Crowbar never asserts
	// a death it has not observed. Then the exit reconcile lands it Exited.
	f.term.exit(t, oldTerm)
	f.wait()
	_, err = f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "the PTY's death carries the runner away (Exited)")

	// (5) reopening revives the REAL conversation: the last provider, resumed into its
	// own native session, exactly where the user left it.
	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "revive brings back the provider that was last here")

	require.Equal(t, 2, f.term.callCount(), "resume spawns a fresh CLI on the same chat")
	assert.Equal(t, "sid-claude-native", argAfter(t, f.term.calls[1].argv, "--resume"),
		"revive must resume the CLI's OWN conversation, not start a blank one")
}

// TestStopChat_AbortsInFlightTurn_DoesNotWait is the user's explicit choice made testable:
// "close = stop immediately". Unlike SwitchProvider, StopChat must NOT wait for the
// in-flight turn — it terminates the CLI mid-answer and clears the spinner right away.
// The proof is a NEGATIVE (the close never parks on the turn), taken at a moment the test
// knows the close has run: a straight-line call that returns.
func TestStopChat_AbortsInFlightTurn_DoesNotWait(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	parked := parkedOnTurn(t)

	// Straight-line: no goroutine. A switch would PARK here (waiting for the turn) and
	// this call would never return; a close aborts the turn and returns immediately.
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	select {
	case <-parked:
		t.Fatal("close = stop immediately: StopChat must never wait for the in-flight turn")
	default:
	}

	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "the mid-turn CLI is terminated — the abort is intended")
	assert.False(t, f.chat(t, chatID).Working, "the aborted turn's spinner is cleared at once")
}

// TestStopChat_AlreadyDormant_IsNilNoop: a chat whose CLI is already gone has nothing to
// stop, so the close is a clean no-op — never an error, and it terminates nothing.
func TestStopChat_AlreadyDormant_IsNilNoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI exits on its own
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "stopping an already-dormant chat is a nil no-op")
	f.wait()

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	assert.NotEmpty(t, f.chat(t, chatID).ID, "the chat is still there — a no-op close does not remove it")
}

// TestStopChat_TerminateFailure_StillDropsToDormant proves the ordering that makes a close
// reliable: DISPLACE FIRST, terminate best-effort. Even when the SIGTERM genuinely fails,
// the chat is already dormant (the placement fact was recorded before the kill), and the
// close does not wedge — a failed terminate leaks a process that dies on its own, never a
// close the user asked for.
func TestStopChat_TerminateFailure_StillDropsToDormant(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "a best-effort close never fails on a terminate error")
	f.wait()

	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "the terminate was still ATTEMPTED")
	_, err := f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "displace-first means the chat is dormant even when the kill fails")
}

// TestStopChat_DoesNotDeleteTheChat pins the boundary against PurgeChat: a close STOPS the
// process but must never Forget the chat — it stays in the read model, addressable and
// resumable, exactly the difference between closing a tab and deleting a chat.
func TestStopChat_DoesNotDeleteTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	got := f.chat(t, chatID)
	assert.Equal(t, chatID, got.ID, "a close must not delete the chat")

	all, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	var found bool
	for _, c := range all {
		if c.ID == chatID {
			found = true
		}
	}
	assert.True(t, found, "the stopped chat still appears in the chat list")
}
