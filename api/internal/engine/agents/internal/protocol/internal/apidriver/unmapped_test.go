package apidriver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// A server request the descriptor does not map gets a method-not-found error at
// once, so the remote side fails fast; an unmapped notification is simply dropped.
func TestStart_AnUnmappedServerRequestIsRefusedAsMethodNotFound(t *testing.T) {
	d, err := descriptor.ParseV0([]byte(`
id: neutral
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [x]
    handshake: { call: hello }
  spawn:
    cmd: x
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
`))
	require.NoError(t, err)
	in, out, peer := testutil.StdioPipe(t)
	replies := make(chan map[string]json.RawMessage, 1)
	go func() {
		var hello struct {
			ID json.RawMessage `json:"id"`
		}
		if peer.ReadJSON(&hello) != nil || peer.WriteJSON(map[string]any{"id": hello.ID, "result": map[string]string{}}) != nil {
			return
		}
		if peer.WriteJSON(map[string]any{"method": "unmapped/note", "params": map[string]string{}}) != nil ||
			peer.WriteJSON(map[string]any{"id": 9, "method": "unmapped/ask", "params": map[string]string{}}) != nil {
			return
		}
		var reply map[string]json.RawMessage
		if peer.ReadJSON(&reply) == nil {
			replies <- reply
		}
	}()

	drv, err := apidriver.Start(t.Context(), d, jsonrpc.Stdio{In: in, Out: out}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })

	select {
	case reply := <-replies:
		require.Equal(t, "9", string(reply["id"]), "the notification earned no reply")
		require.JSONEq(t, `{"code":-32601,"message":"method not found: unmapped/ask"}`, string(reply["error"]))
	case <-time.After(waitFor):
		t.Fatal("the unmapped request was never answered")
	}
}
