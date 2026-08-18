package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// TestInjectedPrompts_DecodeFromADescriptorBlock reads the block claude ships, so
// the key names in the YAML and the tags on the struct cannot drift apart
// silently — a renamed key would decode as an EMPTY list, which is the shape that
// means "every prompt is the user's" and would put the defect straight back.
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

// TestInjectedPromptKinds_HoldsEveryDeclaredConstant: the closed set is what the
// validator checks a descriptor's `kind` against, so a constant missing from it
// would be rejected in the very descriptor that introduced it.
func TestInjectedPromptKinds_HoldsEveryDeclaredConstant(t *testing.T) {
	assert.Contains(t, spec.InjectedPromptKinds, spec.InjectedPromptTaskNotification)
	assert.Len(t, spec.InjectedPromptKinds, 1,
		"the set grows only with a payload captured from a real CLI")
}

// TestInjectedPrompts_AbsentBlockDecodesToNothing pins the degradation default in
// the type itself: a descriptor that says nothing about injected prompts declares
// none, and a provider declaring none records every user_prompt as the user's.
func TestInjectedPrompts_AbsentBlockDecodesToNothing(t *testing.T) {
	var d spec.Descriptor

	require.NoError(t, yaml.Unmarshal([]byte("id: probe\n"), &d))

	assert.Empty(t, d.InjectedPrompts)
}
