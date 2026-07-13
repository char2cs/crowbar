package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestMapHook_ClaudeTurnStop(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{
		"session_id":             "abc-123",
		"last_assistant_message": "acknowledged",
	})
	require.NoError(t, err)
	require.Equal(t, "turn_stop", ev.Kind)
	require.Equal(t, "abc-123", ev.SessionID)
	require.Equal(t, "acknowledged", ev.Message)
}

func TestMapHook_ClaudeUserPrompt(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("user_prompt", map[string]any{"prompt": "hello there"})
	require.NoError(t, err)
	require.Equal(t, "hello there", ev.Message)
}

func TestMapHook_DottedPathDescent(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: nested
spawn: { cmd: x, interactive_required: true }
hooks:
  format: json
  events:
    session_start: { session_id: session.id }
    turn_stop: { message: result.text }
`))
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{
		"result": map[string]any{"text": "deep"},
	})
	require.NoError(t, err)
	require.Equal(t, "deep", ev.Message)
}

func TestMapHook_MissingFieldYieldsEmpty(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("turn_stop", map[string]any{"session_id": "s"})
	require.NoError(t, err)
	require.Equal(t, "", ev.Message)
}

func TestMapHook_UnknownCanonicalErrors(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	_, err = d.MapHook("nope", map[string]any{})
	require.Error(t, err)
}
