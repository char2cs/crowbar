package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The payloads below are CAPTURED from claude 2.1.236 through a real PTY, not
// composed to make a mapping pass. The turn they come from is the one that proves
// why message_delta exists: the agent said ALPHA, ran a Bash tool, then said
// OMEGA, and turn_stop reported OMEGA alone.
const (
	alphaDeltaZero = `{"session_id":"57000ce3","transcript_path":"/tmp/t.jsonl","cwd":"/w",
	  "hook_event_name":"MessageDisplay","prompt_id":"d0918585","turn_id":"0933df18",
	  "message_id":"7cca2d0c","index":0,"final":false,
	  "delta":"ALPHA\n\n1. Apples grow on deciduous trees in the rose family.\n"}`
	alphaDeltaFinal = `{"session_id":"57000ce3","transcript_path":"/tmp/t.jsonl","cwd":"/w",
	  "hook_event_name":"MessageDisplay","prompt_id":"d0918585","turn_id":"0933df18",
	  "message_id":"7cca2d0c","index":3,"final":true,
	  "delta":"5. Cut flesh browns quickly from oxidation."}`
	omegaDeltaZero = `{"session_id":"57000ce3","transcript_path":"/tmp/t.jsonl","cwd":"/w",
	  "hook_event_name":"MessageDisplay","prompt_id":"d0918585","turn_id":"0933df18",
	  "message_id":"a5c55cf9","index":0,"final":false,
	  "delta":"OMEGA\n\n1. Oranges are citrus fruits with a leathery, oil-rich rind.\n"}`
)

func TestAgent_ParseHookMapsAMessageDelta(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookMessageDelta, []byte(alphaDeltaZero))

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "0933df18", ev.Delta.TurnID)
	assert.Equal(t, "7cca2d0c", ev.Delta.MessageID)
	assert.Equal(t, 0, ev.Delta.Index)
	assert.False(t, ev.Delta.Final)
	assert.Equal(t,
		"ALPHA\n\n1. Apples grow on deciduous trees in the rose family.\n",
		ev.Delta.Text)
}

// Final is the terminator, and reading it wrongly is what would leave every
// message open or close every message at once.
func TestAgent_ParseHookReadsTheFinalDelta(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookMessageDelta, []byte(alphaDeltaFinal))

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, 3, ev.Delta.Index)
	assert.True(t, ev.Delta.Final)
}

// TestAgent_ParseHookSeparatesTwoMessagesOfOneTurn is the defect this kind exists
// for, expressed as a test. Both messages carry the SAME turn_id and DIFFERENT
// message_ids, and index restarts at zero — so a reader groups by turn, assembles
// by message, and orders by index. Merging on turn_id alone would concatenate
// ALPHA and OMEGA into one message that the agent never said.
func TestAgent_ParseHookSeparatesTwoMessagesOfOneTurn(t *testing.T) {
	claude := get(t, "claude")

	alpha, err := claude.ParseHook(agents.HookMessageDelta, []byte(alphaDeltaZero))
	require.NoError(t, err)
	omega, err := claude.ParseHook(agents.HookMessageDelta, []byte(omegaDeltaZero))
	require.NoError(t, err)

	assert.Equal(t, alpha.Delta.TurnID, omega.Delta.TurnID)
	assert.NotEqual(t, alpha.Delta.MessageID, omega.Delta.MessageID)
	assert.Equal(t, 0, omega.Delta.Index, "index restarts within each message")
}

// Captured from the subagent's positive control: the CLI pointed at a dead
// endpoint, which fires StopFailure in 1.6s. This is the shape Crowbar must read
// to tell a failed turn from an agent still working.
const stopFailurePayload = `{"session_id":"57000ce3","transcript_path":"/tmp/t.jsonl","cwd":"/w",
  "hook_event_name":"StopFailure","error":"server_error",
  "last_assistant_message":"API Error: Connection refused — a firewall or proxy may be blocking it (ConnectionRefused)"}`

func TestAgent_ParseHookMapsAFailedTurn(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookTurnFailed, []byte(stopFailurePayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Failure)
	assert.Equal(t, "server_error", ev.Failure.Reason)
	assert.Contains(t, ev.Message, "Connection refused")
}

// error_details is optional and was absent on the captured payload. A provider
// omitting it must not produce a broken event — the reason alone is the point.
func TestAgent_ParseHookToleratesAFailedTurnWithNoDetail(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookTurnFailed, []byte(stopFailurePayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Failure)
	assert.Empty(t, ev.Failure.Detail)
}

// codex declares neither kind — it has no streaming hook at all, and its eleven
// events are already fully mapped. It must therefore report both as undeclared
// rather than as empty, which is what keeps a provider without the concept
// behaving exactly as it did before the concept existed.
func TestAgent_CodexDeclaresNeitherStreamingNorFailureHooks(t *testing.T) {
	codex := get(t, "codex")

	for _, kind := range []string{agents.HookMessageDelta, agents.HookTurnFailed} {
		_, err := codex.ParseHook(kind, []byte(`{"session_id":"s"}`))

		require.Errorf(t, err, "codex must not claim to observe %s", kind)
	}
}
