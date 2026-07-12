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

	// claude reads an appended system prompt without being given a turn, and
	// honours a FRESH one on --resume too, so the same silent channel carries
	// {context} whether the session is new or resumed.
	require.Len(t, d.ContextInject, 1)
	require.Equal(t, "pass_arg", d.ContextInject[0].Verb)
	require.Equal(t, "--append-system-prompt", d.ContextInject[0].Args["arg"])
	require.Equal(t, "{context}", d.ContextInject[0].Args["value"])

	require.Len(t, d.ResumeContextInject, 1)
	require.Equal(t, "pass_arg", d.ResumeContextInject[0].Verb)
	require.Equal(t, "--append-system-prompt", d.ResumeContextInject[0].Args["arg"])
	require.Equal(t, "{context}", d.ResumeContextInject[0].Args["value"])
}

func TestResolveDescriptor_EmbeddedCodexValid(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Equal(t, "codex", d.ID)
	require.Contains(t, d.Spawn.ForbidFlags, "exec")
	require.Contains(t, d.Spawn.Args, "--dangerously-bypass-hook-trust")

	// codex ships no --append-system-prompt, and its positional [PROMPT] IS the
	// user's first message — putting {context} there is what made codex answer
	// the handoff on sight. A FRESH codex session takes it silently through the
	// documented `developer_instructions` config key instead.
	require.Len(t, d.ContextInject, 1)
	require.Equal(t, "pass_arg", d.ContextInject[0].Verb)
	require.Equal(t, "-c", d.ContextInject[0].Args["arg"])
	require.Equal(t, "developer_instructions={context}", d.ContextInject[0].Args["value"])

	// A RESUMED codex session cannot be told anything through config (verified
	// against 0.139.0: `codex resume` rebuilds from a rollout that never records
	// developer instructions, `codex fork` behaves the same, and AGENTS.md is not
	// re-read), so the ONLY channel left is a USER MESSAGE — and what it carries is a
	// POINTER at the ledger already on disk, never the transcript, which would dump a
	// wall of handed-off text into the chat on every switch.
	require.Len(t, d.ResumeContextInject, 1)
	require.Equal(t, "pass_arg", d.ResumeContextInject[0].Verb)
	require.Equal(t, "{context_pointer}", d.ResumeContextInject[0].Args["positional"])
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

func TestLoadDescriptor_ParsesDisplayMetadata(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`id: demo
display_name: Demo Provider
icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M1 1h1v1H1z"/></svg>'
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Equal(t, "Demo Provider", d.DisplayName)
	require.Contains(t, d.Icon, "currentColor")
}

func TestValidate_DisplayFieldsAreOptional(t *testing.T) {
	// A descriptor with NO icon/display_name still validates: the display-only
	// carve-out must not break the "every engine field load-bearing" invariant.
	d, err := agent.LoadDescriptor([]byte(`id: bare
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`))
	require.NoError(t, err)
	require.Empty(t, d.Icon)
	require.Empty(t, d.DisplayName)
}

// TestResolveDescriptor_ProvidersOpenInAutoMode: a Crowbar chat is a working agent
// pane, so both CLIs open in their AUTO mode rather than the default
// ask-before-every-action one — being prompted per edit makes the pane useless.
// Neither uses the "skip every check" escape hatch (claude's bypassPermissions,
// codex's --dangerously-bypass-approvals-and-sandbox): codex still runs inside a
// workspace-write sandbox and escalates when it needs to leave it.
func TestResolveDescriptor_ProvidersOpenInAutoMode(t *testing.T) {
	claude, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Contains(t, claude.Spawn.Args, "--permission-mode")
	require.Contains(t, claude.Spawn.Args, "auto")
	require.NotContains(t, claude.Spawn.Args, "bypassPermissions")
	require.NotContains(t, claude.Spawn.Args, "--dangerously-skip-permissions")

	codex, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	require.Contains(t, codex.Spawn.Args, "--sandbox")
	require.Contains(t, codex.Spawn.Args, "workspace-write")
	require.Contains(t, codex.Spawn.Args, "--ask-for-approval")
	require.Contains(t, codex.Spawn.Args, "on-request")
	require.NotContains(t, codex.Spawn.Args, "--dangerously-bypass-approvals-and-sandbox")
}
