package prompt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/prompt"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

func agentState(
	systemPrompt string,
) flow.StateDefinition {
	return flow.StateDefinition{
		Name:  "test",
		Agent: &flow.AgentDef{SystemPrompt: systemPrompt},
	}
}

func TestBuild_noPriorOutputs(
	t *testing.T,
) {
	state := agentState("Do the thing.")
	got := prompt.Build(state, "Build a feature", nil)
	assert.Contains(t, got, "Do the thing.")
	assert.Contains(t, got, "Build a feature")
	assert.NotContains(t, got, "Prior step outputs")
}

func TestBuild_includesTaskTitle(
	t *testing.T,
) {
	state := agentState("Do the thing.")
	got := prompt.Build(state, "Add a hello.go file", nil)
	assert.Contains(t, got, "--- Task ---")
	assert.Contains(t, got, "Add a hello.go file")
}

func TestBuild_includesOperatingInstructions(
	t *testing.T,
) {
	state := agentState("Do the thing.")
	got := prompt.Build(state, "Build a feature", nil)
	assert.Contains(t, got, "crowbar_signal")
	assert.Contains(t, got, "autonomously")
}

func TestBuild_withPriorOutputs(
	t *testing.T,
) {
	state := agentState("Do the thing.")
	priors := []prompt.AgentRunOutput{
		{StateName: "brainstorming", Output: "some ideas"},
		{StateName: "spec", Output: "a spec"},
	}
	got := prompt.Build(state, "Build a feature", priors)
	assert.Contains(t, got, "Do the thing.")
	assert.Contains(t, got, "--- Prior step outputs ---")
	assert.Contains(t, got, "[brainstorming] some ideas")
	assert.Contains(t, got, "[spec] a spec")
}

func TestBuild_nilAgent(
	t *testing.T,
) {
	state := flow.StateDefinition{Name: "human_review"}
	got := prompt.Build(state, "Build a feature", nil)
	assert.Equal(t, "", got)
}

func TestBuild_emptyPriorOutputs(
	t *testing.T,
) {
	state := agentState("Do the thing.")
	got := prompt.Build(state, "Build a feature", []prompt.AgentRunOutput{})
	assert.Contains(t, got, "Do the thing.")
	assert.NotContains(t, got, "Prior step outputs")
}
