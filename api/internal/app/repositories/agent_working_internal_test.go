package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWorkingContainer builds the bare Container state the agent-turn overlay
// touches (the mutex + the in-memory set). The overlay is deliberately a plain
// map guarded by mu — no read model, no event store — so it can be exercised
// directly, with no timing and no I/O.
func newWorkingContainer() *Container {
	return &Container{agentWorking: map[string]map[string]struct{}{}}
}

// TestSetAgentTurn_OnlyReportsEmptinessFlips is the guard for the git-subprocess
// storm: registerAgentWorkingProjection rebroadcasts a workspace ONLY when
// setAgentTurn reports true, and a rebroadcast costs a `git merge-tree` under the
// per-clone git mutex (enrichFrame → eligibilityFor). Working is derived from
// `len(set) > 0`, so redundant turn events inside an already-working workspace
// must report false and cost nothing.
func TestSetAgentTurn_OnlyReportsEmptinessFlips(t *testing.T) {
	c := newWorkingContainer()

	// First chat starts a turn: idle → working. This is a real transition.
	require.True(t, c.setAgentTurn("ws1", "chatA", true), "first turn must flip the overlay on")
	assert.True(t, c.agentWorkingFor("ws1"))

	// A SECOND chat in the SAME workspace starts a turn. The workspace was already
	// working, so nothing observable changed — no rebroadcast, no git subprocess.
	assert.False(t, c.setAgentTurn("ws1", "chatB", true),
		"a concurrent second turn must NOT trigger a redundant rebroadcast")
	assert.True(t, c.agentWorkingFor("ws1"))

	// Re-delivery of a turn_started for a chat already mid-turn is likewise inert.
	assert.False(t, c.setAgentTurn("ws1", "chatA", true), "re-marking the same chat must not flip")

	// chatA stops, but chatB is still mid-turn: the workspace stays working, so
	// again no rebroadcast — and, critically, the overlay must NOT go false here.
	assert.False(t, c.setAgentTurn("ws1", "chatA", false),
		"stopping one of two overlapping turns must NOT flip the overlay")
	assert.True(t, c.agentWorkingFor("ws1"), "workspace still has chatB mid-turn")

	// chatB stops — the LAST turn. working → idle: the transition the FE needs.
	assert.True(t, c.setAgentTurn("ws1", "chatB", false), "the final turn must flip the overlay off")
	assert.False(t, c.agentWorkingFor("ws1"))

	// A stray turn_stopped for an already-idle workspace is inert.
	assert.False(t, c.setAgentTurn("ws1", "chatB", false), "stopping an already-idle chat must not flip")
	assert.False(t, c.agentWorkingFor("ws1"))
}

// TestSetAgentTurn_WorkspacesAreIndependent proves the set is keyed per workspace:
// a turn in ws2 flips only ws2 (and so rebroadcasts only ws2's tree row).
func TestSetAgentTurn_WorkspacesAreIndependent(t *testing.T) {
	c := newWorkingContainer()

	require.True(t, c.setAgentTurn("ws1", "chatA", true))
	require.True(t, c.setAgentTurn("ws2", "chatB", true), "a first turn in another workspace flips that one")

	assert.True(t, c.agentWorkingFor("ws1"))
	assert.True(t, c.agentWorkingFor("ws2"))

	require.True(t, c.setAgentTurn("ws1", "chatA", false))
	assert.False(t, c.agentWorkingFor("ws1"))
	assert.True(t, c.agentWorkingFor("ws2"), "ws2's turn is unaffected by ws1 going idle")
}

// TestSetAgentTurn_ForgetMidTurnFlipsOff covers the delete path: OnForget clears a
// chat that was mid-turn, which must flip the overlay off (a delete may never wedge
// the spinner on) — but only when it was the workspace's last live turn.
func TestSetAgentTurn_ForgetMidTurnFlipsOff(t *testing.T) {
	c := newWorkingContainer()
	require.True(t, c.setAgentTurn("ws1", "chatA", true))
	require.False(t, c.setAgentTurn("ws1", "chatB", true))

	// Forgetting chatA while chatB still works: no flip, overlay stays on.
	assert.False(t, c.setAgentTurn("ws1", "chatA", false))
	assert.True(t, c.agentWorkingFor("ws1"))

	// Forgetting chatB, the last mid-turn chat: flips off.
	assert.True(t, c.setAgentTurn("ws1", "chatB", false))
	assert.False(t, c.agentWorkingFor("ws1"))
}

// TestSetAgentTurn_EmptySetIsPruned keeps the map from growing without bound: a
// workspace with no mid-turn chats holds no entry at all.
func TestSetAgentTurn_EmptySetIsPruned(t *testing.T) {
	c := newWorkingContainer()
	c.setAgentTurn("ws1", "chatA", true)
	c.setAgentTurn("ws1", "chatA", false)
	assert.NotContains(t, c.agentWorking, "ws1", "an emptied workspace set must be deleted, not left behind")
}
