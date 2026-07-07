package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestResolveDescriptor_EmbeddedClaudeValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Equal(t, "claude", d.ID)
	require.Equal(t, "claude", d.Spawn.Cmd)
	require.True(t, d.Spawn.InteractiveRequired)
	require.Contains(t, d.Spawn.ForbidFlags, "-p")
	require.Equal(t, "json", d.Hooks.Format)
	require.Equal(t, "session_id", d.Hooks.Events["session_start"]["session_id"])
	require.Equal(t, "last_assistant_message", d.Hooks.Events["turn_stop"]["message"])

	// Task 9: claude's system_prompt_inject delivers the title instruction via
	// the same real --append-system-prompt flag as handoff_inject, distinct
	// mechanisms that happen to render to the same flag for this provider.
	require.Len(t, d.SystemPromptInject, 1)
	require.Equal(t, "pass_arg", d.SystemPromptInject[0].Verb)
	require.Equal(t, "--append-system-prompt", d.SystemPromptInject[0].Args["arg"])
	require.Equal(t, "{system_prompt}", d.SystemPromptInject[0].Args["value"])
}

func TestResolveDescriptor_EmbeddedCodexValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Equal(t, "codex", d.ID)
	require.Contains(t, d.Spawn.ForbidFlags, "exec")
	require.Contains(t, d.Spawn.Args, "--dangerously-bypass-hook-trust")

	// Task 9: codex has no system-prompt flag, so system_prompt_inject writes
	// the title instruction to $CODEX_HOME/AGENTS.md instead of hijacking
	// codex's positional initial-prompt arg (handoff_inject's mechanism).
	require.Len(t, d.SystemPromptInject, 1)
	require.Equal(t, "write_file", d.SystemPromptInject[0].Verb)
	require.Equal(t, "{tmp}/codex-home/AGENTS.md", d.SystemPromptInject[0].Args["path"])
	require.Equal(t, "{system_prompt}", d.SystemPromptInject[0].Args["content"])
}

func TestLoadDescriptor_RejectsMissingID(t *testing.T) {
	_, err := agent.LoadDescriptor([]byte("spawn:\n  cmd: x\n"))
	require.Error(t, err)
}

func TestParsePayload_JSON(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	m, err := d.ParsePayload([]byte(`{"session_id":"x"}`))
	require.NoError(t, err)
	require.Equal(t, "x", m["session_id"])
}

func TestParsePayload_UnknownFormatErrors(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: p
spawn: { cmd: x, interactive_required: true }
hooks:
  format: toml
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: msg }
`))
	require.NoError(t, err)
	_, err = d.ParsePayload([]byte("x=1"))
	require.Error(t, err)
}
