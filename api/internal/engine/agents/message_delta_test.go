package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

func TestAgent_ParseHookReadsTheFinalDelta(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookMessageDelta, []byte(alphaDeltaFinal))

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, 3, ev.Delta.Index)
	assert.True(t, ev.Delta.Final)
}

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

func TestAgent_ParseHookToleratesAFailedTurnWithNoDetail(t *testing.T) {
	ev, err := get(t, "claude").ParseHook(agents.HookTurnFailed, []byte(stopFailurePayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Failure)
	assert.Empty(t, ev.Failure.Detail)
}

// codex still declares no failure hook at all — turn_failed has no wire event
// on either transport.
func TestAgent_CodexDeclaresNoFailureHook(t *testing.T) {
	_, err := get(t, "codex").ParseHook(agents.HookTurnFailed, []byte(`{"session_id":"s"}`))

	require.Error(t, err, "codex must not claim to observe turn_failed")
}

// message_delta IS observed now — over the api transport (item/agentMessage/delta),
// not a hook. See the mixed transport design spec.
func TestAgent_CodexObservesMessageDeltaOverAPITransport(t *testing.T) {
	ev, err := get(t, "codex").ParseHook(agents.HookMessageDelta,
		[]byte(`{"threadId":"t1","itemId":"m1","turnId":"tn1","delta":"partial text"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Delta)
	assert.Equal(t, "partial text", ev.Delta.Text)
}
