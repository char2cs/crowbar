package apidriver_test

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
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("../descriptor/descriptors-v3/codex.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

// fakeCodexServer replays a scripted sequence of frames after completing the
// initialize handshake exactly as the real app-server does — see wsrpc's own
// tests for the base handshake plumbing; this adds the initialized notification
// step apidriver.Start performs before handing back a Driver.
func fakeCodexServer(t *testing.T, afterInit func(*websocket.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "apidriver")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
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
				var req struct {
					ID json.RawMessage `json:"id"`
				}
				require.NoError(t, json.Unmarshal(msg, &req))
				resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]string{}})
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))

				_, _, err = conn.ReadMessage() // initialized notification, discarded
				require.NoError(t, err)

				afterInit(conn)
			}),
		},
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Close()
		_ = os.Remove(sockPath)
	})
	return sockPath
}

func TestStart_HandshakeThenDeliversCanonicalEvents(t *testing.T) {
	turnCompleted, err := os.ReadFile("../../testdata/fixtures/codex/turn_completed.json")
	require.NoError(t, err)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	select {
	case ev := <-drv.Events():
		require.Equal(t, "turn_stop", ev.Canonical)
		require.Nil(t, ev.AskID)
		require.Contains(t, string(ev.Raw), "threadId")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the dispatched event")
	}
}

func TestStart_AsksCarryAReplyChannel(t *testing.T) {
	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		ask, _ := json.Marshal(map[string]any{
			"id": 7, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(msg, &reply))
		require.Equal(t, 7, reply.ID)
		require.JSONEq(t, `{"decision":"approved"}`, string(reply.Result))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	ev := <-drv.Events()
	require.Equal(t, "permission", ev.Canonical)
	require.NotNil(t, ev.AskID)
	require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"approved"}`)))
}

func TestStart_MalformedParamsAreDroppedNotFatal(t *testing.T) {
	turnCompleted, err := os.ReadFile("../../testdata/fixtures/codex/turn_completed.json")
	require.NoError(t, err)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		// A frame whose params is not a JSON object at all — must be skipped,
		// not crash the translate loop, and the NEXT valid frame must still land.
		bad, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": "not-an-object"})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, bad))
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
		_, _, _ = conn.ReadMessage()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	select {
	case ev := <-drv.Events():
		require.Equal(t, "turn_stop", ev.Canonical)
	case <-time.After(5 * time.Second):
		t.Fatal("the valid frame after the malformed one never arrived")
	}
}

func TestStart_MissingHandshakeCallIsAnError(t *testing.T) {
	raw := []byte(`
id: no-handshake
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [x]
  spawn:
    cmd: x
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
`)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = apidriver.Start(ctx, d, "/nonexistent.sock")
	require.Error(t, err)
}
