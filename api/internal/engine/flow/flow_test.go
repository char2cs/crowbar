package flow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validFlowYAML is a small flow that passes both schema and semantic validation.
// NOTE: the NoTransitionToTerminal rule fires when a non-terminal state transitions
// into a terminal state. To avoid it, we have two non-terminal states forming a
// cycle and a standalone terminal, which keeps all validator rules happy.
const validFlowYAML = `
name: disk-flow
version: "1.0"
states:
  - name: alpha
    ui: chat
    transitions:
      - on: next
        to: beta
  - name: beta
    ui: chat
    transitions:
      - on: back
        to: alpha
  - name: done
    terminal: true
`

// validFlowYAMLWithTerminalEntry is the minimal form needed when you want to
// allow a transition into the terminal state (which triggers the
// NoTransitionToTerminal validation rule and causes Load to fail).
// We keep validFlowYAML free of that to exercise the happy path.

const invalidFlowYAMLMissingName = `
states:
  - name: start
    terminal: true
`

const invalidFlowYAMLBadUI = `
name: bad
states:
  - name: start
    ui: badmode
    terminal: true
`

// --- Load (package-level function) ---

func TestLoad_EmptyPath_ReturnsBuiltin(t *testing.T) {
	fd, err := flow.Load("")
	require.NoError(t, err)
	assert.Equal(t, "feature-development", fd.Name)
}

func TestLoad_BuiltinKeyword_ReturnsBuiltin(t *testing.T) {
	fd, err := flow.Load("builtin:feature-development")
	require.NoError(t, err)
	assert.Equal(t, "feature-development", fd.Name)
}

func TestLoad_FilePath_ParsesAndValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validFlowYAML), 0o600))

	fd, err := flow.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "disk-flow", fd.Name)
}

func TestLoad_NonExistentFile_ReturnsError(t *testing.T) {
	_, err := flow.Load("/tmp/this-file-absolutely-does-not-exist-crowbar.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow loader: read")
}

func TestLoad_InvalidYAML_SchemaError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte(invalidFlowYAMLMissingName), 0o600))

	_, err := flow.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow loader: parse")
}

func TestLoad_BadUIValue_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_ui.yaml")
	require.NoError(t, os.WriteFile(path, []byte(invalidFlowYAMLBadUI), 0o600))

	_, err := flow.Load(path)
	require.Error(t, err)
}

func TestLoad_ValidationFails_ReturnsError(t *testing.T) {
	// Write a YAML that passes schema but fails semantic validation:
	// a non-terminal state with no transitions.
	yaml := `
name: invalid-semantic
states:
  - name: orphan
`
	dir := t.TempDir()
	path := filepath.Join(dir, "semantic_fail.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := flow.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow loader: validation failed")
}

// --- NewLoader (interface implementation) ---

func TestNewLoader_EmptyPath_ReturnsBuiltin(t *testing.T) {
	l := flow.NewLoader()
	fd, err := l.Load("")
	require.NoError(t, err)
	assert.Equal(t, "feature-development", fd.Name)
}

func TestNewLoader_BuiltinKeyword_ReturnsBuiltin(t *testing.T) {
	l := flow.NewLoader()
	fd, err := l.Load("builtin:feature-development")
	require.NoError(t, err)
	assert.Equal(t, "feature-development", fd.Name)
}

func TestNewLoader_FilePath_ParsesAndValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validFlowYAML), 0o600))

	l := flow.NewLoader()
	fd, err := l.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "disk-flow", fd.Name)
}

func TestNewLoader_NonExistentFile_ReturnsError(t *testing.T) {
	l := flow.NewLoader()
	_, err := l.Load("/tmp/this-file-absolutely-does-not-exist-crowbar-loader.yaml")
	require.Error(t, err)
}

// --- Exported type aliases are accessible ---

func TestFlowPackage_ConstantsAccessible(t *testing.T) {
	// Verify the re-exported constants compile and have expected values.
	assert.Equal(t, flow.UIMode("chat"), flow.UIModeChat)
	assert.Equal(t, flow.UIMode("kanban"), flow.UIModeKanban)
	assert.Equal(t, flow.UIMode("diff"), flow.UIModeDiff)
	assert.Equal(t, flow.UIMode("background"), flow.UIModeBackground)

	assert.Equal(t, flow.IntelligenceLevel("low"), flow.IntelligenceLow)
	assert.Equal(t, flow.IntelligenceLevel("medium"), flow.IntelligenceMedium)
	assert.Equal(t, flow.IntelligenceLevel("high"), flow.IntelligenceHigh)
	assert.Equal(t, flow.IntelligenceLevel("ultrahigh"), flow.IntelligenceUltrahigh)

	assert.Equal(t, flow.FlowTool("crowbar.signal"), flow.ToolCrowbarSignal)
	assert.Equal(t, flow.FlowTool("crowbar.create_item"), flow.ToolCrowbarCreateItem)
	assert.Equal(t, flow.FlowTool("crowbar.update_item_status"), flow.ToolCrowbarUpdateItemStatus)
	assert.Equal(t, flow.FlowTool("crowbar.get_items"), flow.ToolCrowbarGetItems)
	assert.Equal(t, flow.FlowTool("crowbar.open_thread"), flow.ToolCrowbarOpenThread)
	assert.Equal(t, flow.FlowTool("crowbar.reply_thread"), flow.ToolCrowbarReplyThread)
	assert.Equal(t, flow.FlowTool("crowbar.get_threads"), flow.ToolCrowbarGetThreads)
	assert.Equal(t, flow.FlowTool("crowbar.resolve_thread"), flow.ToolCrowbarResolveThread)
	assert.Equal(t, flow.FlowTool("fs.read"), flow.ToolFSRead)
	assert.Equal(t, flow.FlowTool("fs.write"), flow.ToolFSWrite)
	assert.Equal(t, flow.FlowTool("terminal"), flow.ToolTerminal)
}
