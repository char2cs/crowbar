package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
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

func declaredPrompts(t *testing.T, block string) *spec.Descriptor {
	t.Helper()
	d := valid()
	require.NoError(t, yaml.Unmarshal([]byte("injected_prompts:\n"+block), d))
	return d
}

func TestInjectedPrompts_AcceptsAnAnchoredMatchAndItems(t *testing.T) {
	d := declaredPrompts(t, `
  - kind: task_notification
    match: '^\[BATCH \d+ done'
    items:
      - pattern: '- (?P<id>\w+): exit_code=(?P<exit_code>\d+)'
  - kind: task_notification
    needle: "<task-notification>"
    task_id: { pattern: '<task-id>(?P<v>[^<]+)</task-id>' }
`)

	require.NoError(t, rules.Apply(d))
}

func TestInjectedPrompts_RejectsTheBrokenMatchAndItemsShapes(t *testing.T) {
	testCases := map[string]string{
		"a match unanchored would hit a person's sentence": `
  - match: '\[BATCH \d+ done'`,
		"a needle and a match together": `
  - needle: "[BATCH"
    match: '^\[BATCH \d+ done'`,
		"a task_id and items together": `
  - kind: task_notification
    needle: "[BATCH"
    task_id: { pattern: 'task (?P<v>\w+)' }
    items:
      - pattern: '- (?P<id>\w+):'`,
	}
	for name, block := range testCases {
		t.Run(name, func(t *testing.T) {
			err := rules.Apply(declaredPrompts(t, block))

			require.Error(t, err)
			assert.ErrorIs(t, err, rules.ErrInvalidDescriptor)
		})
	}

	t.Run("an items entry that never loaded names no id", func(t *testing.T) {
		d := valid()
		d.InjectedPrompts = []spec.InjectedPromptSpec{
			{Kind: spec.InjectedPromptTaskNotification, Needle: "[BATCH", Items: []spec.TaskPattern{{}}},
		}

		assert.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
	})
	t.Run("a match that never loaded can match nothing", func(t *testing.T) {
		d := valid()
		d.InjectedPrompts = []spec.InjectedPromptSpec{{Match: &spec.PromptMatch{}}}

		assert.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
	})
}

func TestInjectedPrompts_AbsentIsValid(t *testing.T) {
	d := valid()
	d.InjectedPrompts = nil

	require.NoError(t, rules.Apply(d))
}
