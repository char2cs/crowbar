package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

// validMinimalDescriptor satisfies Validate on its own; individual tests below
// append a config_injection block or strip a required field to isolate one
// failure branch at a time.
const validMinimalDescriptor = `
id: testprov
spawn:
  cmd: testcmd
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`

func TestLoadDescriptor_RejectsMalformedYAML(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte("id: [unterminated\n  nested: {\n"))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsMissingSpawnCmd(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn:
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsInteractiveRequiredFalse(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn:
  cmd: testcmd
  interactive_required: false
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsMissingSessionStartHook(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn:
  cmd: testcmd
  interactive_required: true
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsSessionStartMissingSessionIDField(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsMissingHooksFormat(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: last_assistant_message }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsTurnStopMissingMessage(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(`
id: testprov
spawn: { cmd: testcmd, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { }
`))
	require.Error(t, err)
}

func TestLoadDescriptor_RejectsInjectStepWithMultipleVerbs(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(validMinimalDescriptor + `
config_injection:
  - set_env: { name: A, value: "1" }
    pass_arg: { arg: "--x" }
`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "exactly one verb")
}

func TestLoadDescriptor_RejectsInjectStepThatIsNotAMapping(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte(validMinimalDescriptor + `
config_injection:
  - just_a_scalar_string
`))
	require.Error(t, err)
}

func TestResolveDescriptor_UnknownProviderReturnsError(t *testing.T) {
	_, err := agent.ResolveDescriptor(t.TempDir(), "totally-unknown-provider")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown provider")
}

// TestResolveDescriptor_DiskOverrideWinsOverEmbedded exercises the disk
// override branch: a descriptor at <home>/descriptors/<id>.yaml must win over
// (and never even reach) the embedded default.
func TestResolveDescriptor_DiskOverrideWinsOverEmbedded(t *testing.T) {
	home := t.TempDir()
	descDir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(descDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(descDir, "custom.yaml"), []byte(validMinimalDescriptor), 0o644))

	d, err := agent.ResolveDescriptor(home, "custom")
	require.NoError(t, err)
	require.Equal(t, "testprov", d.ID) // came from the disk override, not an embedded default
}
