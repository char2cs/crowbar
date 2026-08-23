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
			name:    "an empty needle silences every user message",
			prompts: []spec.InjectedPromptSpec{{Needle: ""}},
		},
		{
			name:    "a whitespace-only needle silences every user message",
			prompts: []spec.InjectedPromptSpec{{Needle: " \n\t"}},
		},
		{
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

func TestInjectedPrompts_AbsentIsValid(t *testing.T) {
	d := valid()
	d.InjectedPrompts = nil

	require.NoError(t, rules.Apply(d))
}
