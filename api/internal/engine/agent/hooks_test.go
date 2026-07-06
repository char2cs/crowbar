package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestMapHook_ClaudeSessionStart(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	ev, err := d.MapHook("session_start", map[string]any{
		"session_id":      "abc-123",
		"transcript_path": "/x/abc-123.jsonl",
	})
	require.NoError(t, err)
	require.Equal(t, "session_start", ev.Kind)
	require.Equal(t, "abc-123", ev.SessionID)
	require.Equal(t, "/x/abc-123.jsonl", ev.Transcript)
}

func TestMapHook_UnknownCanonicalErrors(t *testing.T) {
	d, _ := agent.ResolveDescriptor(t.TempDir(), "claude")
	_, err := d.MapHook("nope", map[string]any{})
	require.Error(t, err)
}
