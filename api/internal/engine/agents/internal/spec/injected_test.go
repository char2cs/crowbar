package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestInjectedPrompts_DecodeFromADescriptorBlock(t *testing.T) {
	var d spec.Descriptor

	require.NoError(t, yaml.Unmarshal([]byte(`
id: probe
injected_prompts:
  - kind: task_notification
    needle: "<task-notification>"
  - needle: "<unnamed-injection>"
`), &d))

	require.Len(t, d.InjectedPrompts, 2)
	assert.Equal(t, spec.InjectedPromptTaskNotification, d.InjectedPrompts[0].Kind)
	assert.Equal(t, "<task-notification>", d.InjectedPrompts[0].Needle)
	assert.Empty(t, d.InjectedPrompts[1].Kind, "a needle may decline to name a kind")
}

func TestInjectedPromptKinds_HoldsEveryDeclaredConstant(t *testing.T) {
	assert.Contains(t, spec.InjectedPromptKinds, spec.InjectedPromptTaskNotification)
	assert.Contains(t, spec.InjectedPromptKinds, spec.InjectedPromptSubagentResult)
	assert.Len(t, spec.InjectedPromptKinds, 2,
		"the set grows only with a payload captured from a real CLI")
}

func TestInjectedPrompts_AbsentBlockDecodesToNothing(t *testing.T) {
	var d spec.Descriptor

	require.NoError(t, yaml.Unmarshal([]byte("id: probe\n"), &d))

	assert.Empty(t, d.InjectedPrompts)
}

func TestInjectedPrompts_ASubagentResultReadsItsFieldsOutOfThePrompt(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
id: probe
injected_prompts:
  - kind: subagent_result
    needle: "<agent-message"
    scope_id: { pattern: 'from="(?P<v>[^"]+)"' }
`), &d))

	p := d.InjectedPrompts[0]
	assert.Equal(t, "a-1", p.ScopeID.Find(`<agent-message from="a-1">done</agent-message>`))
	assert.Empty(t, p.ScopeID.Find("no sender"))
	assert.Empty(t, p.Result.Find("anything"), "an undeclared field reads nothing")

	err := yaml.Unmarshal([]byte("injected_prompts:\n  - scope_id: { pattern: 'no group' }\n"), &d)
	assert.Error(t, err, "a pattern keeps its (?P<v>...) group")
}
