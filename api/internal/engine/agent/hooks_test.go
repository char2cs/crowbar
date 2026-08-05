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

// The payloads below are VERBATIM captures from codex 0.146.0 running with
// `[features] memories = true`, recorded by standing in for `crowbar hook` on the
// real `-c hooks.*` commands Crowbar injects. The two arrived 6 seconds apart out of
// ONE codex process: the first is the user's conversation, the second is codex's own
// Memory Writing Agent (Phase 2 consolidation) running as an internal session.
//
// They are kept whole, and kept together, because the point they make is a
// comparison: every field the move vocabulary looks at is identical (`source:
// startup`, a well-formed fresh `session_id`), and the only structural difference is
// that the internal one has no rollout. Trimming them to the fields the assertions
// read would delete the evidence.
const (
	codexRealSessionStart = `{
  "session_id": "019fafae-f27f-7d00-bd23-3bcbce0ece1f",
  "transcript_path": "/Users/u/.codex/sessions/2026/07/29/rollout-2026-07-29T18-01-46-019fafae-f27f-7d00-bd23-3bcbce0ece1f.jsonl",
  "cwd": "/tmp/ws",
  "hook_event_name": "SessionStart",
  "model": "gpt-5.6-sol",
  "permission_mode": "default",
  "source": "startup"
}`
	codexMemorySessionStart = `{
  "session_id": "019fafaf-4f2c-7551-806e-eda96d1cefed",
  "transcript_path": null,
  "cwd": "/Users/u/.codex/memories",
  "hook_event_name": "SessionStart",
  "model": "gpt-5.6-terra",
  "permission_mode": "bypassPermissions",
  "source": "startup"
}`
)

// TestOwnsConversation_CodexInternalMemorySession is the engine-level half of the
// codex-memories fix: the descriptor's require_payload_fields must separate the
// user's conversation from codex's internal memory session, and must separate them on
// the ONE field that actually differs in kind.
func TestOwnsConversation_CodexInternalMemorySession(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)

	real, err := d.ParsePayload([]byte(codexRealSessionStart))
	require.NoError(t, err)
	missing, owns := d.OwnsConversation(real)
	require.True(t, owns, "the user's own conversation must be ingested; %q was read as absent", missing)

	memory, err := d.ParsePayload([]byte(codexMemorySessionStart))
	require.NoError(t, err)
	missing, owns = d.OwnsConversation(memory)
	require.False(t, owns, "codex's internal memory session must not be taken for a conversation")
	require.Equal(t, "transcript_path", missing)

	// And pin WHY it cannot be told apart any other way — so that a future attempt to
	// swap this guard for a cheaper label test fails here rather than in production.
	require.Equal(t, real["source"], memory["source"],
		"source is identical across the two, which is why Decide must not branch on it")
	require.NotEqual(t, real["session_id"], memory["session_id"])
}

// TestOwnsConversation_UndeclaredIsUnaffected pins the safe default: a provider that
// declares no require_payload_fields keeps exactly its previous behaviour, so adding
// the guard cannot have changed claude.
func TestOwnsConversation_UndeclaredIsUnaffected(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	require.Empty(t, d.Hooks.RequirePayloadFields)

	_, owns := d.OwnsConversation(map[string]any{})
	require.True(t, owns, "a provider that requires nothing must accept everything it did before")
}

// TestOwnsConversation_RequiresEveryDeclaredField pins the AND: one satisfied field
// does not carry a payload past a second, unsatisfied one.
func TestOwnsConversation_RequiresEveryDeclaredField(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(`
id: twofield
spawn: { cmd: x, interactive_required: true }
hooks:
  format: json
  require_payload_fields: [transcript_path, session.owner]
  events:
    session_start: { session_id: session.id }
    turn_stop: { message: result.text }
`))
	require.NoError(t, err)

	missing, owns := d.OwnsConversation(map[string]any{"transcript_path": "/x.jsonl"})
	require.False(t, owns)
	require.Equal(t, "session.owner", missing, "a dotted path must be walked, not compared literally")

	_, owns = d.OwnsConversation(map[string]any{
		"transcript_path": "/x.jsonl",
		"session":         map[string]any{"owner": "user"},
	})
	require.True(t, owns)
}
