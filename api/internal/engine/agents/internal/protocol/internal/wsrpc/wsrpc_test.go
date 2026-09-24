package wsrpc_test

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

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
)

// serveOnUnixSocket starts an HTTP server listening on a unix socket (not TCP),
// upgrading every connection to a WebSocket and handing it to serve. codex's own
// app-server does exactly this over `--listen unix://PATH` — confirmed live
// against codex-cli 0.146.0 (see the design spec's "Corrections" section): a raw
// NDJSON writer gets `httparse error: invalid token` and the connection is
// closed with no response, because the socket speaks WebSocket, not bare
// newline-delimited JSON like `--listen stdio://` does.
func serveOnUnixSocket(t *testing.T, serve func(*websocket.Conn)) string {
	t.Helper()
	// A short path under the system temp root, NOT t.TempDir(): that nests under
	// the test binary's own working directory with the full (sometimes long)
	// test name embedded, and macOS's sun_path is a hard 104 bytes — a name like
	// TestCall_ServerErrorSurfacesAsAGoError already overflows it as a t.TempDir().
	dir, err := os.MkdirTemp("", "wsrpc")
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
				serve(conn)
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

func TestDialAndCall_RoundTrips(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		require.Equal(t, "initialize", req.Method)
		resp, _ := json.Marshal(map[string]any{
			"id":     req.ID,
			"result": map[string]string{"codexHome": "/fake"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	result, err := conn.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "crowbar"}})
	require.NoError(t, err)
	require.JSONEq(t, `{"codexHome":"/fake"}`, string(result))
}

func TestCall_ServerErrorSurfacesAsAGoError(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		resp, _ := json.Marshal(map[string]any{
			"id":    req.ID,
			"error": map[string]any{"code": -32600, "message": "invalid request"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Call(ctx, "thread/start", map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid request")
}

func TestFrames_DeliversNotificationsAndAsksButNotOwnCallResponses(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		// A notification (no id).
		note, _ := json.Marshal(map[string]any{
			"method": "thread/started",
			"params": map[string]string{"threadId": "t1"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, note))
		// A server-initiated ask (has both id and method).
		ask, _ := json.Marshal(map[string]any{
			"id": 99, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		// Read our reply to that ask and never send it back out as a Frame.
		_, replyMsg, err := conn.ReadMessage()
		require.NoError(t, err)
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(replyMsg, &reply))
		require.Equal(t, 99, reply.ID)
		require.JSONEq(t, `{"decision":"approved"}`, string(reply.Result))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	frame1 := <-conn.Frames()
	require.Equal(t, "thread/started", frame1.Method)
	require.Nil(t, frame1.ID)

	frame2 := <-conn.Frames()
	require.Equal(t, "item/permissions/requestApproval", frame2.Method)
	require.NotNil(t, frame2.ID)

	require.NoError(t, conn.Reply(frame2.ID, json.RawMessage(`{"decision":"approved"}`)))
}

// A consumer that is not draining notifications must never hold up the
// response to one of our own calls (Stop's interrupt, a prompt's turn/start).
func TestCall_IsAnsweredWhileNotificationsBackUp(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		for i := 0; i < 500; i++ {
			note, _ := json.Marshal(map[string]any{"method": "item/agentMessage/delta", "params": map[string]int{"i": i}})
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, note))
		}
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]string{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // hold the connection until the client leaves
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Call(ctx, "turn/interrupt", map[string]any{})
	require.NoError(t, err, "the response must not queue behind undelivered notifications")

	first := <-conn.Frames()
	require.Equal(t, "item/agentMessage/delta", first.Method, "nothing read is lost")
	require.False(t, conn.Overflowed())
}

// Past its bound the mailbox does not grow without limit: the connection is
// closed and reports why, so its runner ends with a recorded cause.
func TestFrames_OverflowClosesTheConnection(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		for i := 0; i < 50; i++ {
			note, _ := json.Marshal(map[string]any{"method": "item/agentMessage/delta", "params": map[string]int{"i": i}})
			if conn.WriteMessage(websocket.TextMessage, note) != nil {
				return
			}
		}
		_, _, _ = conn.ReadMessage()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath, wsrpc.WithMaxQueued(10))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Nothing drains until the mailbox has overflowed.
	require.Eventually(t, conn.Overflowed, 3*time.Second, 5*time.Millisecond)
	delivered := 0
	for range conn.Frames() {
		delivered++
	}
	require.LessOrEqual(t, delivered, 1, "an overflow drops what was queued and closes the stream")
}

func TestNotify_SendsNoID(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var frame map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(msg, &frame))
		_, hasID := frame["id"]
		require.False(t, hasID, "a notification must carry no id")
		require.JSONEq(t, `"turn/interrupt"`, string(frame["method"]))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.Notify("turn/interrupt", map[string]string{"threadId": "t1"}))
}

func TestDial_UnreachableSocketIsAnError(t *testing.T) {
	dir, err := os.MkdirTemp("", "wsrpc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = wsrpc.Dial(ctx, filepath.Join(dir, "nope.sock"))
	require.Error(t, err)
}

func TestClose_UnblocksAPendingCall(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		// Read the request but never reply, then hang until the client closes.
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
	})

	ctx := context.Background()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, callErr := conn.Call(ctx, "thread/start", map[string]any{})
		done <- callErr
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, conn.Close())

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Call did not unblock after Close")
	}
}
