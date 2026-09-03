package wsrpc_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
)

// TestCall_UnmarshalableParams_FailsBeforeAnyWrite covers Call's own
// json.Marshal(params) failure — a channel value can never round-trip through
// encoding/json — which must be reported without ever touching the socket.
func TestCall_UnmarshalableParams_FailsBeforeAnyWrite(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		// The server must never see a request: a bad Call never reaches the wire.
		_, _, _ = conn.ReadMessage()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Call(ctx, "initialize", map[string]any{"bad": make(chan int)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal params")
}

// TestNotify_UnmarshalableParams_FailsBeforeAnyWrite is Notify's counterpart.
func TestNotify_UnmarshalableParams_FailsBeforeAnyWrite(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.Notify("progress", map[string]any{"bad": make(chan int)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal params")
}

// TestCall_AfterClose_ReturnsConnectionClosedWithoutBlocking covers the guard
// at the top of Call's critical section: a Call issued after Close must fail
// immediately with the connection-closed sentinel message, not hang waiting
// for a reply that will never arrive.
func TestCall_AfterClose_ReturnsConnectionClosedWithoutBlocking(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	_, err = conn.Call(ctx, "initialize", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

// TestCall_ContextCanceled_UnblocksWithContextError covers Call's ctx.Done()
// branch: the server accepts the request but deliberately never replies, so
// the only way Call returns is the caller's own context being canceled.
func TestCall_ContextCanceled_UnblocksWithContextError(t *testing.T) {
	received := make(chan struct{})
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		_, _, err := conn.ReadMessage()
		require.NoError(t, err)
		close(received)
		// Deliberately never reply — this Call must be unblocked by ctx
		// cancellation, not by a response. Block here until the client hangs up
		// (ReadMessage then errors and this handler goroutine returns).
		_, _, _ = conn.ReadMessage()
	})
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()
	conn, err := wsrpc.Dial(dialCtx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	callCtx, callCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, callErr := conn.Call(callCtx, "initialize", nil)
		errCh <- callErr
	}()

	<-received
	callCancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Call did not unblock on context cancellation")
	}
}

// TestReadLoop_MalformedFrame_IsDroppedNotFatal covers readLoop's
// json.Unmarshal failure branch: a frame that is not valid JSON must be
// silently dropped, and the connection must keep working for the next,
// well-formed frame — a single garbled message must never kill the connection.
func TestReadLoop_MalformedFrame_IsDroppedNotFatal(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("not json at all")))
		notif, _ := json.Marshal(map[string]any{"method": "progress", "params": map[string]string{"ok": "yes"}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, notif))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	select {
	case f := <-conn.Frames():
		assert.Equal(t, "progress", f.Method, "the malformed frame ahead of it must be dropped, not delivered or fatal")
	case <-time.After(5 * time.Second):
		t.Fatal("well-formed frame after a malformed one was never delivered")
	}
}

// TestDispatchResponse_NonNumericID_IsDroppedNotFatal covers dispatchResponse's
// own json.Unmarshal failure: a response frame whose id is not a bare number
// (never produced by THIS client, but not guaranteed of every peer) must be
// dropped rather than panicking or wedging the read loop.
func TestDispatchResponse_NonNumericID_IsDroppedNotFatal(t *testing.T) {
	sockPath := serveOnUnixSocket(t, func(conn *websocket.Conn) {
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))

		bad, _ := json.Marshal(map[string]any{"id": "not-a-number", "result": map[string]string{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, bad))

		real, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]string{"ok": "yes"}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, real))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := wsrpc.Dial(ctx, sockPath)
	require.NoError(t, err)
	defer conn.Close()

	result, err := conn.Call(ctx, "initialize", nil)

	require.NoError(t, err, "the bogus-id frame must be dropped, letting the real response still resolve the Call")
	require.JSONEq(t, `{"ok":"yes"}`, string(result))
}
