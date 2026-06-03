package translator_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
	"github.com/char2cs/crowbar/api/internal/engine/flow/translator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalYAML is a valid flow with all required fields.
const minimalYAML = `
name: test-flow
version: "1.0"
description: A minimal test flow
item_statuses:
  - todo
  - done
states:
  - name: start
    ui: chat
    terminal: false
    items: false
    agent:
      intelligence: medium
      system_prompt: "do things"
      tools:
        - fs.read
        - terminal
    transitions:
      - on: done
        to: finish
    emits:
      - agent: improver
        on: done
  - name: finish
    terminal: true
`

func TestParse_ValidYAML(t *testing.T) {
	fd, err := translator.Parse([]byte(minimalYAML))
	require.NoError(t, err)
	assert.Equal(t, "test-flow", fd.Name)
	assert.Equal(t, "1.0", fd.Version)
	assert.Equal(t, "A minimal test flow", fd.Description)
	assert.Equal(t, []string{"todo", "done"}, fd.ItemStatuses)
	require.Len(t, fd.States, 2)

	start := fd.States[0]
	assert.Equal(t, "start", start.Name)
	assert.False(t, start.Terminal)
	assert.Equal(t, def.UIModeChat, start.UI)
	assert.False(t, start.Items)
	require.NotNil(t, start.Agent)
	assert.Equal(t, def.IntelligenceMedium, start.Agent.Intelligence)
	assert.Equal(t, "do things", start.Agent.SystemPrompt)
	assert.Equal(t, []def.FlowTool{"fs.read", "terminal"}, start.Agent.Tools)
	require.Len(t, start.Transitions, 1)
	assert.Equal(t, "done", start.Transitions[0].On)
	assert.Equal(t, "finish", start.Transitions[0].To)
	require.Len(t, start.Emits, 1)
	assert.Equal(t, "improver", start.Emits[0].Agent)
	assert.Equal(t, "done", start.Emits[0].On)

	finish := fd.States[1]
	assert.Equal(t, "finish", finish.Name)
	assert.True(t, finish.Terminal)
}

func TestParse_MissingName_SchemaError(t *testing.T) {
	yaml := `
states:
  - name: start
    terminal: true
`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema validation")
}

func TestParse_MissingStates_SchemaError(t *testing.T) {
	yaml := `
name: no-states
`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema validation")
}

func TestParse_BadUIValue_SchemaError(t *testing.T) {
	yaml := `
name: bad-ui
states:
  - name: start
    ui: fakemode
    terminal: true
`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema validation")
}

func TestParse_BadIntelligenceValue_SchemaError(t *testing.T) {
	yaml := `
name: bad-intel
states:
  - name: start
    terminal: true
    agent:
      intelligence: genius
`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema validation")
}

func TestParse_InvalidYAML_ParseError(t *testing.T) {
	yaml := `name: [unclosed`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
}

func TestParse_NoAgent_StateHasNilAgent(t *testing.T) {
	yaml := `
name: human-only
states:
  - name: review
    terminal: true
`
	fd, err := translator.Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, fd.States, 1)
	assert.Nil(t, fd.States[0].Agent)
}

func TestParse_StateWithItemsTrue(t *testing.T) {
	yaml := `
name: kanban-flow
states:
  - name: impl
    ui: kanban
    items: true
    terminal: true
`
	fd, err := translator.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.True(t, fd.States[0].Items)
}

func TestParse_AllUIAndIntelligenceModes(t *testing.T) {
	tests := []struct {
		name          string
		ui            string
		intelligence  string
		expectedUI    def.UIMode
		expectedIntel def.IntelligenceLevel
	}{
		{"chat-low", "chat", "low", def.UIModeChat, def.IntelligenceLow},
		{"kanban-medium", "kanban", "medium", def.UIModeKanban, def.IntelligenceMedium},
		{"diff-high", "diff", "high", def.UIModeDiff, def.IntelligenceHigh},
		{"background-ultrahigh", "background", "ultrahigh", def.UIModeBackground, def.IntelligenceUltrahigh},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := "name: test\nstates:\n  - name: s\n    terminal: true\n    ui: " + tc.ui + "\n    agent:\n      intelligence: " + tc.intelligence + "\n"
			fd, err := translator.Parse([]byte(yaml))
			require.NoError(t, err)
			assert.Equal(t, tc.expectedUI, fd.States[0].UI)
			assert.Equal(t, tc.expectedIntel, fd.States[0].Agent.Intelligence)
		})
	}
}

func TestParse_EmptyStatesArray_SchemaError(t *testing.T) {
	yaml := `
name: empty-states
states: []
`
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
}

func TestParse_NonStringMapKey_JSONConversionError(t *testing.T) {
	// YAML with integer keys produces map[interface{}]interface{} from yaml.v3.
	// convertYAMLNode passes it through the default case unchanged, causing
	// json.Marshal to fail inside toJSONCompatible.
	yaml := "1: foo\n2: bar"
	_, err := translator.Parse([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yaml→json conversion")
}

func TestParse_EmptyInput(t *testing.T) {
	_, err := translator.Parse([]byte(""))
	require.Error(t, err)
}

func TestParse_MultipleTransitionsAndEmits(t *testing.T) {
	yaml := `
name: multi
states:
  - name: a
    transitions:
      - on: e1
        to: b
      - on: e2
        to: c
    emits:
      - agent: bot1
        on: e1
      - agent: bot2
  - name: b
    terminal: true
  - name: c
    terminal: true
`
	fd, err := translator.Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, fd.States[0].Transitions, 2)
	require.Len(t, fd.States[0].Emits, 2)
	assert.Equal(t, "e1", fd.States[0].Transitions[0].On)
	assert.Equal(t, "b", fd.States[0].Transitions[0].To)
	assert.Equal(t, "bot1", fd.States[0].Emits[0].Agent)
	assert.Equal(t, "bot2", fd.States[0].Emits[1].Agent)
	assert.Equal(t, "", fd.States[0].Emits[1].On)
}
