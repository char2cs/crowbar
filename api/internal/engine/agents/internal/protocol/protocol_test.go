package protocol_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("internal/descriptor/descriptors-v3/codex.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

// scriptedCall is one JSON-RPC request the fake app-server answers, in order.
type scriptedCall struct {
	method string
	result string
}

// scriptedServer completes the api-transport handshake exactly as
// apidriver.Start expects, then replays script's call/response pairs in
// order, recording each call's raw params so a test can assert on the actual
// wire payload APIConn's facade produced.
func scriptedServer(t *testing.T, script []scriptedCall) (sockPath string, seenParams *[]string) {
	t.Helper()
	seen := make([]string, 0, len(script))
	dir, err := os.MkdirTemp("", "protocolfacade")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	upgrader := websocket.Upgrader{}
	srv := &httptest.Server{
		Listener: ln,
		Config: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				require.NoError(t, err)
				defer conn.Close()

				_, msg, err := conn.ReadMessage() // initialize
				require.NoError(t, err)
				var initReq struct {
					ID json.RawMessage `json:"id"`
				}
				require.NoError(t, json.Unmarshal(msg, &initReq))
				resp, _ := json.Marshal(map[string]any{"id": initReq.ID, "result": map[string]string{}})
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
				_, _, err = conn.ReadMessage() // initialized notification, discarded
				require.NoError(t, err)

				for _, step := range script {
					_, msg, err := conn.ReadMessage()
					require.NoError(t, err)
					var req struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
					}
					require.NoError(t, json.Unmarshal(msg, &req))
					require.Equal(t, step.method, req.Method, "unexpected call reaching the wire")
					seen = append(seen, string(req.Params))
					resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": json.RawMessage(step.result)})
					require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
				}
				_, _, _ = conn.ReadMessage() // block until the client closes
			}),
		},
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Close()
		_ = os.Remove(sock)
	})
	return sock, &seen
}

// TestAPIConn_FacadeForwardsEveryCallThroughToTheUnderlyingDriver pins that
// protocol.APIConn — the only api-transport handle callers outside this
// package's own internal/ boundary may hold — actually forwards ctx,
// canonical and values to *apidriver.Driver and hands back its answers
// unchanged. apidriver's own tests already prove the DRIVER's behaviour;
// nothing exercises THIS facade's wiring, so a dropped argument or a
// swapped method on APIConn's own EstablishSession/Dispatch/InjectAt would
// ship silently.
func TestAPIConn_FacadeForwardsEveryCallThroughToTheUnderlyingDriver(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"sid-1"}}`},
		{method: "thread/inject_items", result: `{}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(ctx, d, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	out, err := conn.EstablishSession(ctx, "prompt", map[string]string{"session_id": "sid-1", "cwd": "/work"})
	require.NoError(t, err)
	assert.Equal(t, "sid-1", out["session_id"], "APIConn.EstablishSession must hand back what the driver resolved")

	err = conn.InjectAt(ctx, "context", map[string]string{
		"session_id": "sid-1", "context": "while you were away, this happened",
	})
	require.NoError(t, err)
	require.Len(t, *seen, 2, "APIConn.InjectAt must reach the wire as its own call")
	assert.Contains(t, (*seen)[1], `"threadId":"sid-1"`, "InjectAt must forward the session the caller named")
	assert.Contains(t, (*seen)[1], "while you were away, this happened", "and the context text itself")

	out, err = conn.Dispatch(ctx, "prompt", map[string]string{"session_id": "sid-1", "cwd": "/work", "text": "hi"})
	require.NoError(t, err)
	assert.Equal(t, "sid-1", out["session_id"], "APIConn.Dispatch must hand back what the driver resolved")
	require.Len(t, *seen, 3, "Dispatch on an already-established session skips straight to the action call")
}

// TestAPIConn_InjectAtUndeclaredMomentIsANoopThroughTheFacade proves the
// no-wire-call contract for an undeclared inject moment survives being
// reached through APIConn rather than the driver directly.
func TestAPIConn_InjectAtUndeclaredMomentIsANoopThroughTheFacade(t *testing.T) {
	sockPath, seen := scriptedServer(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(ctx, d, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.InjectAt(ctx, "resume", map[string]string{"session_id": "sid-1"})
	require.NoError(t, err)
	assert.Empty(t, *seen, "an undeclared moment must never reach the wire")
}

// TestStartAPIDriver_PropagatesTheUnderlyingStartFailure pins the error path:
// StartAPIDriver must hand back (nil, err) rather than a half-built
// connection when the underlying handshake cannot even begin.
func TestStartAPIDriver_PropagatesTheUnderlyingStartFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(ctx, d, "/nonexistent.sock")

	require.Error(t, err)
	assert.Nil(t, conn)
}

// TestSends_ListsOnlyEventsDeclaringAnOutboundCall pins protocol.Sends'
// delegation to outbound.Declared: an event with only an inbound `in:` must
// not be reported as something Crowbar can send.
func TestSends_ListsOnlyEventsDeclaringAnOutboundCall(t *testing.T) {
	d := &spec.Descriptor{
		Events: map[string]spec.EventSpec{
			"compact_start": {Out: "thread/compact/start"},
			"turn_stop":     {In: "turn/completed"},
		},
	}

	assert.Equal(t, []string{"compact_start"}, protocol.Sends(d))
}

// TestCheckVersion_DelegatesToTheDescriptorsDeclaredRange pins protocol.
// CheckVersion's delegation to descriptor.CheckProtocolVersion.
func TestCheckVersion_DelegatesToTheDescriptorsDeclaredRange(t *testing.T) {
	d := &spec.Descriptor{ProtocolVersion: &spec.VersionRange{Min: "1.0", Max: "1.9"}}

	assert.NoError(t, protocol.CheckVersion(d, "1.5"))
	assert.Error(t, protocol.CheckVersion(d, "2.0"),
		"a version outside the descriptor's declared range must be refused")
}
