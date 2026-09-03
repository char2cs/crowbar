//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_GracefulShutdown_MidTurn_ClosesAbandonedTurn is the guard for a
// chat that spins FOREVER, across restarts, because the daemon was stopped politely.
//
// A graceful stop kills every vendor CLI's PTY, and a dying CLI's exit callback is
// what records the death: it Exits the runner AND closes the turn the CLI abandoned
// (nothing else will — the turn_stop hook died with the process). But that callback
// runs on the terminal engine's REAP goroutine, and the shutdown sequence used to
// close the DBs without ever waiting for it. The race was lost in production, and it
// announced itself in the log as the reap path failing against a store already gone:
//
//	WARN agent: close abandoned turn: … err="sql: database is closed"
//
// (that line read "get chat" when it was captured; closeAbandonedTurn no longer reads
// the chat at all — the surviving warning is "abandon turn". The failure is the same.)
//
// Losing it is worse than a lost log line, because the two writes fall on
// opposite sides of the close: the runner's Exit COMMITS (its live row is gone),
// while the turn is NOT closed. The chat is left Working — and on the next boot,
// ReconcileRunnersOnBoot finds NO live runner row for that chat, so it has nothing to
// reconcile and nothing ever closes the turn. The chat spins forever, across every
// restart, and the workspace's whole working overlay spins with it.
//
// The assertion is made in the ONE window where the fact is observable: after the
// daemon's ordered graceful drain (harness.drain — HTTP stopped, every asynchronous
// writer quiesced, every aggregate drained) and BEFORE the DBs are closed underneath
// it. That is precisely the ordering under test: the reap path must have COMPLETED by
// then, not merely have been scheduled.
//
// It is deterministic, and it must be: it blocks on the drain itself (a real signal),
// never on a timer. A test that slept "to let the reaper finish" would be asserting
// the bug's absence on a coin flip, and would have passed against the broken code.
//
// This does NOT overlap the boot-reconcile suite. That one pins the CRASH path (the
// power cut, where nothing is recorded and the next boot must repair it). This pins
// the graceful path's OWN obligation: to record the death on the way down, so that
// the next boot has nothing to repair.
func TestRegression_GracefulShutdown_MidTurn_ClosesAbandonedTurn(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, runnerID := bootReconcileFixture(t, home)
	ctx := context.Background()

	// --- the daemon is stopped POLITELY, mid-turn. ---
	h1.drain()

	// The drain has returned, so every writer it is responsible for has finished
	// writing. The DBs are still open: what we read now is exactly what the adapter
	// is about to checkpoint and close.
	chat, err := h1.app.Usecases.AgentChat.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.False(t, chat.Working,
		"a graceful stop must close the turn its own kill abandoned, BEFORE the DBs that hold it close: "+
			"the CLI it SIGTERMed will never send the turn_stop hook, and once the runner's Exit has "+
			"committed there is no live-runner row left for the next boot's reconcile to find — so a turn "+
			"left open here is a chat that spins forever, across every restart")

	_, err = h1.app.Usecases.AgentRunner.LiveRunnerForChat(ctx, chatID)
	assert.Error(t, err,
		"runner %s's PTY was killed by the shutdown, so its live row must be gone: the PTY is the sole "+
			"authority on whether a CLI is alive, and the exit callback that records its death must have "+
			"run before the DBs closed", runnerID)

	// --- and the chat comes back from the restart DORMANT, not spinning. ---
	h1.shutdown()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	post := getAgentChat(t, h2, homeBase, chatID)
	assert.Empty(t, post.LiveRunnerID,
		"a chat whose CLI died with the daemon must come back with no live runner to attach to")
	assert.Equal(t, []string{"sess-before-restart"}, post.sessionIDs(),
		"the chat's conversation history is append-only and describes no process: a graceful stop may not erase it")

	rebooted, err := h2.app.Usecases.AgentChat.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.False(t, rebooted.Working,
		"the closed turn is durable: the chat must not come back spinning")
}
