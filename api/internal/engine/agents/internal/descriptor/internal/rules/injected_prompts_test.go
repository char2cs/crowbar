package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestInjectedPrompts_AcceptsWhatClaudeShips(t *testing.T) {
	d := valid()
	d.InjectedPrompts = []spec.InjectedPromptSpec{
		{Kind: spec.InjectedPromptTaskNotification, Needle: "<task-notification>"},
		{Needle: "<some-unnamed-injection>"},
	}

	require.NoError(t, rules.Apply(d))
}

func TestInjectedPrompts_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		prompts []spec.InjectedPromptSpec
	}{
		{
			// The one failure this feature must never be able to produce: an empty
			// needle is a prefix of every string, so EVERY message the user sends
			// would be filed as the harness's.
			name:    "an empty needle silences every user message",
			prompts: []spec.InjectedPromptSpec{{Needle: ""}},
		},
		{
			// Same failure by a different route: the matcher trims leading whitespace
			// off the prompt first, so a whitespace-only needle is also a prefix of
			// everything.
			name:    "a whitespace-only needle silences every user message",
			prompts: []spec.InjectedPromptSpec{{Needle: " \n\t"}},
		},
		{
			// Can never fire, because the prompt is trimmed before the comparison.
			// Refused rather than trimmed, so a dead declaration is not shipped
			// looking like a live one.
			name:    "a needle with leading whitespace can never match",
			prompts: []spec.InjectedPromptSpec{{Needle: " <task-notification>"}},
		},
		{
			name: "a kind the daemon does not know",
			prompts: []spec.InjectedPromptSpec{
				{Kind: "task_notifcation", Needle: "<task-notification>"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			d.InjectedPrompts = tc.prompts

			err := rules.Apply(d)

			require.Error(t, err)
			assert.ErrorIs(t, err, rules.ErrInvalidDescriptor)
		})
	}
}

// TestInjectedPrompts_AbsentIsValid: declaring nothing is the safe default, and
// every provider that has never been measured is in exactly that state.
func TestInjectedPrompts_AbsentIsValid(t *testing.T) {
	d := valid()
	d.InjectedPrompts = nil

	require.NoError(t, rules.Apply(d))
}
