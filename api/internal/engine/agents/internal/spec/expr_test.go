package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestEventSpec_MapAcceptsAPlainPath(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
`), &d))

	assert.Equal(t, []string{"thread.id"}, d.Events["session_start"].Map["session_id"])
}

func TestEventSpec_MapAcceptsFirstPresent(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { first_present: [item.tool, item.type] }
`), &d))

	assert.Equal(t, []string{"item.tool", "item.type"}, d.Events["tool_pre"].Map["tool_name"])
}

func TestEventSpec_WhenAcceptsAPlainValue(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    when: { item.type: commandExecution }
`), &d))

	assert.Equal(t, []string{"commandExecution"}, d.Events["tool_pre"].When["item.type"])
}

func TestEventSpec_WhenAcceptsAnyOf(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    when: { item.type: { any_of: [commandExecution, fileChange] } }
`), &d))

	assert.Equal(t, []string{"commandExecution", "fileChange"}, d.Events["tool_pre"].When["item.type"])
}

// --- 5.0: the `||` glyph is a hard parse error, everywhere an expression can
// appear, naming the descriptor, event and field it was found in.

func TestEventSpec_MapRejectsRawAlternationGlyph(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: "item.tool || item.type"
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_pre")
	assert.Contains(t, err.Error(), "tool_name")
}

func TestEventSpec_WhenRejectsRawAlternationGlyph(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    when: { item.type: "commandExecution || fileChange" }
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_pre")
	assert.Contains(t, err.Error(), "item.type")
}

func TestEventSpec_FirstPresentRejectsAGlyphWithinTheList(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { first_present: ["item.tool || item.type", item.type] }
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_pre")
}

func TestEventSpec_AnyOfRejectsAGlyphWithinTheList(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    when:
      item.type: { any_of: ["commandExecution || fileChange", mcpToolCall] }
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_pre")
}

func TestEventSpec_WireRefRejectsRawAlternationGlyph(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  permission:
    ask: "item/commandExecution/requestApproval || item/fileChange/requestApproval"
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission")
}

func TestEventSpec_WireRefListRejectsAGlyphWithinAnElement(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  permission:
    ask: ["item/commandExecution/requestApproval || item/fileChange/requestApproval", item/other]
`), &d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission")
}

// first_present: is the map: operator; any_of: is the when: operator. Using the
// wrong one is a descriptor bug the loader must catch, not silently accept.
func TestEventSpec_MapRejectsAnyOf(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { any_of: [a, b] }
`), &d)
	assert.Error(t, err)
}

func TestEventSpec_WhenRejectsFirstPresent(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    when:
      item.type: { first_present: [a, b] }
`), &d)
	assert.Error(t, err)
}

func TestEventSpec_MapRejectsAnEmptyFirstPresentList(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { first_present: [] }
`), &d)
	assert.Error(t, err)
}

func TestEventSpec_MapRejectsMoreThanOneOperatorKey(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { first_present: [a, b], extra: c }
`), &d)
	assert.Error(t, err)
}

func TestEventSpec_MapRejectsANonListOperatorValue(t *testing.T) {
	var d spec.Descriptor
	err := yaml.Unmarshal([]byte(`
events:
  tool_pre:
    in: item/started
    map:
      tool_name: { first_present: not_a_list }
`), &d)
	assert.Error(t, err)
}
