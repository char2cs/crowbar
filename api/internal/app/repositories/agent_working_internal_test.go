package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWorkingContainer() *Container {
	return &Container{agentWorking: map[string]map[string]struct{}{}}
}

func TestSetAgentTurn_OnlyReportsEmptinessFlips(t *testing.T) {
	c := newWorkingContainer()

	require.True(t, c.setAgentTurn("ws1", "chatA", true), "first turn must flip the overlay on")
	assert.True(t, c.agentWorkingFor("ws1"))

	assert.False(t, c.setAgentTurn("ws1", "chatB", true),
		"a concurrent second turn must NOT trigger a redundant rebroadcast")
	assert.True(t, c.agentWorkingFor("ws1"))

	assert.False(t, c.setAgentTurn("ws1", "chatA", true), "re-marking the same chat must not flip")

	assert.False(t, c.setAgentTurn("ws1", "chatA", false),
		"stopping one of two overlapping turns must NOT flip the overlay")
	assert.True(t, c.agentWorkingFor("ws1"), "workspace still has chatB mid-turn")

	assert.True(t, c.setAgentTurn("ws1", "chatB", false), "the final turn must flip the overlay off")
	assert.False(t, c.agentWorkingFor("ws1"))

	assert.False(t, c.setAgentTurn("ws1", "chatB", false), "stopping an already-idle chat must not flip")
	assert.False(t, c.agentWorkingFor("ws1"))
}

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

func TestSetAgentTurn_ForgetMidTurnFlipsOff(t *testing.T) {
	c := newWorkingContainer()
	require.True(t, c.setAgentTurn("ws1", "chatA", true))
	require.False(t, c.setAgentTurn("ws1", "chatB", true))

	assert.False(t, c.setAgentTurn("ws1", "chatA", false))
	assert.True(t, c.agentWorkingFor("ws1"))

	assert.True(t, c.setAgentTurn("ws1", "chatB", false))
	assert.False(t, c.agentWorkingFor("ws1"))
}

func TestSetAgentTurn_EmptySetIsPruned(t *testing.T) {
	c := newWorkingContainer()
	c.setAgentTurn("ws1", "chatA", true)
	c.setAgentTurn("ws1", "chatA", false)
	assert.NotContains(t, c.agentWorking, "ws1", "an emptied workspace set must be deleted, not left behind")
}
