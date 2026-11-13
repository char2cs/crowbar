package apidriver_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
)

// A captured number reaches a later call as a JSON number when its template is exactly
// that one placeholder; a string capture, even a numeric-looking one, stays a string.
func TestSend_ACapturedNumberKeepsItsJSONType(t *testing.T) {
	d, err := descriptor.ParseV0([]byte(`
id: neutral
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [x]
    handshake: { call: hello, notify: ready }
  spawn:
    cmd: x
events:
  prompt:
    fresh:
      - call: open
        capture: { session_id: id }
    action:
      - call: submit
        send: { session: "{session_id}", text: "{text}" }
        capture: { fence: generation, live: active }
  interrupt:
    out: cancel
    send: { session: "{session_id}", fence: "{fence}", live: "{live}", tag: "g{fence}", retries: 2 }
`))
	require.NoError(t, err)
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "open", result: `{"id":"12345"}`},
		{method: "submit", result: `{"generation":7,"active":true}`},
		{method: "cancel", result: `{}`},
	})
	drv, err := apidriver.Start(t.Context(), d, stdio, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	_, err = drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "hi"})
	require.NoError(t, err)
	require.NoError(t, drv.Send(t.Context(), "interrupt", nil))

	require.JSONEq(t, `{"session":"12345","fence":7,"live":true,"tag":"g7","retries":2}`, (*seen)[2])
}
