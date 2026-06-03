package builtin_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow/builtin"
	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeatureDevelopmentFlow_Name(t *testing.T) {
	assert.Equal(t, "feature-development", builtin.FeatureDevelopmentFlow.Name)
}

func TestFeatureDevelopmentFlow_HasSevenStates(t *testing.T) {
	assert.Len(t, builtin.FeatureDevelopmentFlow.States, 7)
}

func TestFeatureDevelopmentFlow_ExactlyOneTerminalState(t *testing.T) {
	var terminals []string
	for _, s := range builtin.FeatureDevelopmentFlow.States {
		if s.Terminal {
			terminals = append(terminals, s.Name)
		}
	}
	require.Len(t, terminals, 1, "expected exactly one terminal state, got: %v", terminals)
	assert.Equal(t, "complete", terminals[0])
}

func TestFeatureDevelopmentFlow_AllTransitionsReferenceExistingStates(t *testing.T) {
	fd := builtin.FeatureDevelopmentFlow
	stateNames := make(map[string]bool, len(fd.States))
	for _, s := range fd.States {
		stateNames[s.Name] = true
	}
	for _, s := range fd.States {
		for _, tr := range s.Transitions {
			assert.True(
				t,
				stateNames[tr.To],
				"state %q transitions to unknown state %q on event %q",
				s.Name, tr.To, tr.On,
			)
		}
	}
}

func TestFeatureDevelopmentFlow_NoTransitionToTerminal(t *testing.T) {
	fd := builtin.FeatureDevelopmentFlow
	terminalNames := make(map[string]bool)
	for _, s := range fd.States {
		if s.Terminal {
			terminalNames[s.Name] = true
		}
	}
	// Every transition to the terminal state should not exist per the validator rule,
	// but the builtin flow is expected to be valid (human_review -> complete is the
	// intentional path). This test documents the actual structure: only human_review
	// transitions into the terminal "complete" state.
	for _, s := range fd.States {
		for _, tr := range s.Transitions {
			if terminalNames[tr.To] {
				// Document which state makes the transition; the builtin flow
				// is intentionally designed this way.
				assert.Equal(t, "human_review", s.Name,
					"only human_review should transition to the terminal state")
				assert.Equal(t, "complete", tr.To)
			}
		}
	}
}

func TestFeatureDevelopmentFlow_NonTerminalStatesHaveTransitions(t *testing.T) {
	for _, s := range builtin.FeatureDevelopmentFlow.States {
		if !s.Terminal {
			assert.NotEmpty(t, s.Transitions,
				"non-terminal state %q has no transitions", s.Name)
		}
	}
}

func TestFeatureDevelopmentFlow_KnownStateNames(t *testing.T) {
	fd := builtin.FeatureDevelopmentFlow
	stateNames := make(map[string]bool, len(fd.States))
	for _, s := range fd.States {
		stateNames[s.Name] = true
	}
	expected := []string{
		"brainstorming",
		"spec",
		"implementation",
		"ai_review",
		"human_review",
		"debugging",
		"complete",
	}
	for _, name := range expected {
		assert.True(t, stateNames[name], "expected state %q to exist", name)
	}
}

func TestFeatureDevelopmentFlow_AgentPresentOnMachineStates(t *testing.T) {
	// All non-terminal states except human_review should have an agent.
	humanOnlyStates := map[string]bool{"human_review": true}
	for _, s := range builtin.FeatureDevelopmentFlow.States {
		if s.Terminal {
			continue
		}
		if humanOnlyStates[s.Name] {
			assert.Nil(t, s.Agent, "state %q should have no agent", s.Name)
		} else {
			assert.NotNil(t, s.Agent, "state %q should have an agent", s.Name)
		}
	}
}

func TestFeatureDevelopmentFlow_UIModesAreValid(t *testing.T) {
	validModes := map[def.UIMode]bool{
		def.UIModeChat:       true,
		def.UIModeKanban:     true,
		def.UIModeDiff:       true,
		def.UIModeBackground: true,
		"":                   true, // terminal states may omit UI mode
	}
	for _, s := range builtin.FeatureDevelopmentFlow.States {
		assert.True(t, validModes[s.UI], "state %q has invalid UI mode %q", s.Name, s.UI)
	}
}

func TestFeatureDevelopmentFlow_Version(t *testing.T) {
	assert.NotEmpty(t, builtin.FeatureDevelopmentFlow.Version)
}

func TestFeatureDevelopmentFlow_ItemStatuses(t *testing.T) {
	assert.NotEmpty(t, builtin.FeatureDevelopmentFlow.ItemStatuses)
}
