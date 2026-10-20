package jsonrpc_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// open speaks over a stdio pipe whose child end is peer.
func open(t *testing.T, opts ...jsonrpc.Option) (*jsonrpc.Conn, *testutil.LinePeer) {
	t.Helper()
	in, out, peer := testutil.StdioPipe(t)
	conn := jsonrpc.New(jsonrpc.Stdio{In: in, Out: out}, opts...)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, peer
}

// serve runs script as the child, on its own goroutine.
func serve(t *testing.T, script func(*testutil.LinePeer), opts ...jsonrpc.Option) *jsonrpc.Conn {
	t.Helper()
	conn, peer := open(t, opts...)
	go script(peer)
	return conn
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

// readRequest is the next request the child reads; safe off the test goroutine.
func readRequest(peer *testutil.LinePeer) (request, error) {
	msg, err := peer.Read()
	if err != nil {
		return request{}, err
	}
	var req request
	return req, json.Unmarshal(msg, &req)
}

func mustReadRequest(t *testing.T, peer *testutil.LinePeer) request {
	t.Helper()
	req, err := readRequest(peer)
	require.NoError(t, err)
	return req
}

func TestCall_RoundTrips(t *testing.T) {
	conn, peer := open(t)
	answered := make(chan json.RawMessage, 1)
	go func() {
		result, err := conn.Call(t.Context(), "initialize", map[string]any{"clientInfo": map[string]string{"name": "crowbar"}})
		assert.NoError(t, err)
		answered <- result
	}()

	req := mustReadRequest(t, peer)
	require.Equal(t, "initialize", req.Method)
	require.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": map[string]string{"home": "/fake"}}))

	require.JSONEq(t, `{"home":"/fake"}`, string(<-answered))
}

// A child is ready only once it answers: a request written before it reads
// waits in the pipe, and nothing answers it in the meantime.
func TestCall_WaitsInThePipeUntilALateChildAnswers(t *testing.T) {
	conn, peer := open(t)
	answered := make(chan error, 1)
	go func() {
		_, err := conn.Call(context.Background(), "initialize", nil)
		answered <- err
	}()

	req := mustReadRequest(t, peer)
	select {
	case err := <-answered:
		t.Fatalf("the call returned before the child answered: %v", err)
	default:
	}
	require.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": map[string]string{}}))
	require.NoError(t, <-answered)
}

// A child that exits closes its stdout: a pending call fails at once, never
// at its timeout.
func TestCall_AChildThatHangsUpFailsTheCallAtOnce(t *testing.T) {
	conn := serve(t, func(peer *testutil.LinePeer) {
		_, _ = peer.Read()
		_ = peer.Close()
	})

	_, err := conn.Call(context.Background(), "initialize", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

func TestCall_ServerErrorSurfacesAsAGoError(t *testing.T) {
	conn := serve(t, func(peer *testutil.LinePeer) {
		req, err := readRequest(peer)
		if !assert.NoError(t, err) {
			return
		}
		assert.NoError(t, peer.WriteJSON(map[string]any{
			"id": req.ID, "error": map[string]any{"code": -32600, "message": "invalid request"},
		}))
	})

	_, err := conn.Call(t.Context(), "thread/start", map[string]any{})

	var callErr *jsonrpc.CallError
	require.ErrorAs(t, err, &callErr)
	assert.Equal(t, -32600, callErr.Code)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestFrames_DeliversNotificationsAndAsksButNotOwnCallResponses(t *testing.T) {
	conn, peer := open(t)
	require.NoError(t, peer.WriteJSON(map[string]any{"method": "thread/started", "params": map[string]string{"threadId": "t1"}}))
	require.NoError(t, peer.WriteJSON(map[string]any{
		"id": 99, "method": "item/permissions/requestApproval", "params": map[string]string{"tool": "shell"},
	}))

	frame1 := <-conn.Frames()
	require.Equal(t, "thread/started", frame1.Method)
	require.Nil(t, frame1.ID)
	frame2 := <-conn.Frames()
	require.Equal(t, "item/permissions/requestApproval", frame2.Method)
	require.NotNil(t, frame2.ID)

	require.NoError(t, conn.Reply(frame2.ID, json.RawMessage(`{"decision":"approved"}`)))
	msg, err := peer.Read()
	require.NoError(t, err)
	var reply struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(msg, &reply))
	require.Equal(t, 99, reply.ID)
	require.JSONEq(t, `{"decision":"approved"}`, string(reply.Result))
}

// A consumer that is not draining notifications must never hold up the
// response to one of our own calls (Stop's interrupt, a prompt's turn/start).
func TestCall_IsAnsweredWhileNotificationsBackUp(t *testing.T) {
	conn := serve(t, func(peer *testutil.LinePeer) {
		req, err := readRequest(peer)
		if !assert.NoError(t, err) {
			return
		}
		for i := 0; i < 500 && err == nil; i++ {
			err = peer.WriteJSON(map[string]any{"method": "item/agentMessage/delta", "params": map[string]int{"i": i}})
		}
		if !assert.NoError(t, err) {
			return
		}
		assert.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": map[string]string{}}))
	})

	_, err := conn.Call(t.Context(), "turn/interrupt", map[string]any{})
	require.NoError(t, err, "the response must not queue behind undelivered notifications")

	first := <-conn.Frames()
	require.Equal(t, "item/agentMessage/delta", first.Method, "nothing read is lost")
	require.False(t, conn.Overflowed())
}

// Past its bound the mailbox does not grow without limit: the connection is
// closed and reports why, so its runner ends with a recorded cause.
func TestFrames_OverflowClosesTheConnection(t *testing.T) {
	conn := serve(t, func(peer *testutil.LinePeer) {
		for i := 0; i < 50; i++ {
			if peer.WriteJSON(map[string]any{"method": "item/agentMessage/delta", "params": map[string]int{"i": i}}) != nil {
				return
			}
		}
	}, jsonrpc.WithMaxQueued(10))

	// Nothing drains until the mailbox has overflowed.
	require.Eventually(t, conn.Overflowed, 3*time.Second, 5*time.Millisecond)
	delivered := 0
	for range conn.Frames() {
		delivered++
	}
	require.LessOrEqual(t, delivered, 1, "an overflow drops what was queued and closes the stream")
}

func TestNotify_SendsNoID(t *testing.T) {
	conn, peer := open(t)

	require.NoError(t, conn.Notify("turn/interrupt", map[string]string{"threadId": "t1"}))

	msg, err := peer.Read()
	require.NoError(t, err)
	var frame map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(msg, &frame))
	_, hasID := frame["id"]
	require.False(t, hasID, "a notification must carry no id")
	require.JSONEq(t, `"turn/interrupt"`, string(frame["method"]))
}

// A value encoding/json cannot marshal is reported before anything reaches
// the child: the next line it reads is the next good message.
func TestCallAndNotify_UnmarshalableParamsFailBeforeAnyWrite(t *testing.T) {
	conn, peer := open(t)

	_, err := conn.Call(t.Context(), "initialize", map[string]any{"bad": make(chan int)})
	require.ErrorContains(t, err, "marshal params")
	require.ErrorContains(t, conn.Notify("progress", map[string]any{"bad": make(chan int)}), "marshal params")

	require.NoError(t, conn.Notify("progress", nil))
	assert.Equal(t, "progress", mustReadRequest(t, peer).Method)
}

func TestCall_AfterClose_ReturnsConnectionClosedWithoutBlocking(t *testing.T) {
	conn, _ := open(t)
	require.NoError(t, conn.Close())

	_, err := conn.Call(t.Context(), "initialize", nil)

	require.ErrorContains(t, err, "connection closed")
}

func TestClose_UnblocksAPendingCall(t *testing.T) {
	conn, peer := open(t)
	done := make(chan error, 1)
	go func() {
		_, err := conn.Call(context.Background(), "thread/start", map[string]any{})
		done <- err
	}()
	mustReadRequest(t, peer) // the call is waiting on an answer that never comes

	require.NoError(t, conn.Close())

	require.Error(t, <-done)
}

// The child accepts the request but never replies: only the caller's own
// context ends the Call.
func TestCall_ContextCanceled_UnblocksWithContextError(t *testing.T) {
	conn, peer := open(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := conn.Call(ctx, "initialize", nil)
		done <- err
	}()
	mustReadRequest(t, peer)

	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
}

// A garbled line never kills the connection: the next well-formed one is
// still delivered.
func TestReadLoop_MalformedLine_IsDroppedNotFatal(t *testing.T) {
	conn, peer := open(t)
	require.NoError(t, peer.Write([]byte("not json at all")))
	require.NoError(t, peer.WriteJSON(map[string]any{"method": "progress", "params": map[string]string{"ok": "yes"}}))

	assert.Equal(t, "progress", (<-conn.Frames()).Method)
}

// A response whose id is not a bare number (never produced by THIS client,
// but not guaranteed of every peer) is dropped rather than wedging the read
// loop, and the real response still resolves the Call.
func TestDispatchResponse_NonNumericID_IsDroppedNotFatal(t *testing.T) {
	conn := serve(t, func(peer *testutil.LinePeer) {
		req, err := readRequest(peer)
		if !assert.NoError(t, err) {
			return
		}
		assert.NoError(t, peer.WriteJSON(map[string]any{"id": "not-a-number", "result": map[string]string{}}))
		assert.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": map[string]string{"ok": "yes"}}))
	})

	result, err := conn.Call(t.Context(), "initialize", nil)

	require.NoError(t, err)
	require.JSONEq(t, `{"ok":"yes"}`, string(result))
}
