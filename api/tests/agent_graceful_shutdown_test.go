//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegression_GracefulShutdown_MidTurn_ClosesAbandonedTurn(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, runnerID := bootReconcileFixture(t, home)
	ctx := context.Background()

	h1.drain()

	assert.False(t, chatBusy(t, h1, chatID),
		"a graceful stop must close the turn its own kill abandoned, BEFORE the DBs that hold it close: "+
			"the CLI it SIGTERMed will never send the turn_stop hook, and once the runner's Exit has "+
			"committed there is no live-runner row left for the next boot's reconcile to find — so a turn "+
			"left open here is a chat that spins forever, across every restart")

	_, err := h1.app.Usecases.AgentRunner.LiveRunnerForChat(ctx, chatID)
	assert.Error(t, err,
		"runner %s's PTY was killed by the shutdown, so its live row must be gone: the PTY is the sole "+
			"authority on whether a CLI is alive, and the exit callback that records its death must have "+
			"run before the DBs closed", runnerID)

	h1.shutdown()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	post := getAgentChat(t, h2, homeBase, chatID)
	assert.Empty(t, post.LiveRunnerID,
		"a chat whose CLI died with the daemon must come back with no live runner to attach to")
	assert.Equal(t, []string{"sess-before-restart"}, post.sessionIDs(),
		"the chat's conversation history is append-only and describes no process: a graceful stop may not erase it")

	assert.False(t, chatBusy(t, h2, chatID),
		"the closed turn is durable: the chat must not come back spinning")
}
