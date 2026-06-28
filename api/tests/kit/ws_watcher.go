//go:build integration

package kit

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// WSWatcher dials a WebSocket topic and provides event-driven WaitFor methods.
// It never uses time.Sleep; all waiting is done via WS read with a deadline.
type WSWatcher struct {
	conn *websocket.Conn
}

// Dial opens a WebSocket connection to rawURL and returns a watcher.
// The connection is closed automatically via t.Cleanup.
func Dial(
	t *testing.T,
	rawURL string,
) *WSWatcher {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(
		rawURL,
		nil,
	)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(
		t,
		err,
	)
	t.Cleanup(func() { _ = conn.Close() })
	return &WSWatcher{conn: conn}
}

func failWSRead(
	t *testing.T,
	err error,
	timeout time.Duration,
) {
	t.Helper()
	var netErr net.Error
	if errors.As(
		err,
		&netErr,
	) && netErr.Timeout() {
		require.NoError(
			t,
			err,
			"ws: no matching message within %v",
			timeout,
		)
	}
	require.NoError(
		t,
		err,
		"ws: ReadMessage failed (connection error)",
	)
}

// ReadUntil reads messages until match returns true or the deadline expires.
// It returns the first matching message. Fails the test on deadline expiry or
// WS read error. Never sleeps.
func (w *WSWatcher) ReadUntil(
	t *testing.T,
	timeout time.Duration,
	match func(msg map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	require.NoError(
		t,
		w.conn.SetReadDeadline(deadline),
		"ws: SetReadDeadline",
	)
	for {
		_, raw, err := w.conn.ReadMessage()
		if err != nil {
			failWSRead(t, err, timeout)
			return nil
		}
		var msg map[string]any
		require.NoError(
			t,
			json.Unmarshal(
				raw,
				&msg,
			),
		)
		if match(msg) {
			return msg
		}
	}
}

// ReadMsg reads a single message within timeout.
func (w *WSWatcher) ReadMsg(
	t *testing.T,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return w.ReadUntil(t, timeout, func(_ map[string]any) bool { return true })
}

// AssertNoMessage asserts that no WebSocket message arrives within timeout.
// It returns true if the read times out cleanly (expected), or fails the test
// if a message arrives before the deadline. It never calls t.Fatal on timeout —
// a timeout is the success condition.
func (w *WSWatcher) AssertNoMessage(
	t *testing.T,
	timeout time.Duration,
) bool {
	t.Helper()
	require.NoError(
		t,
		w.conn.SetReadDeadline(time.Now().Add(timeout)),
		"ws: SetReadDeadline",
	)
	_, _, err := w.conn.ReadMessage()
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// Timeout is expected: no message arrived.
			return true
		}
		require.NoError(
			t,
			err,
			"ws: AssertNoMessage: unexpected connection error",
		)
	}
	t.Fatalf("ws: AssertNoMessage: unexpected message arrived within %v", timeout)
	return false
}

// AssertNoMatch drains messages until timeout and fails the test if any message
// satisfies match. Non-matching messages (legitimate in-prefix frames) are
// ignored. A clean timeout is the success condition. This is the negative
// prefix-filtering idiom: assert an out-of-prefix frame NEVER arrives while
// tolerating the subscriber's own in-prefix traffic. Returns true on success.
func (w *WSWatcher) AssertNoMatch(
	t *testing.T,
	timeout time.Duration,
	match func(msg map[string]any) bool,
) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	require.NoError(
		t,
		w.conn.SetReadDeadline(deadline),
		"ws: SetReadDeadline",
	)
	for {
		_, raw, err := w.conn.ReadMessage()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return true
			}
			require.NoError(
				t,
				err,
				"ws: AssertNoMatch: unexpected connection error",
			)
			return false
		}
		var msg map[string]any
		require.NoError(t, json.Unmarshal(raw, &msg))
		if match(msg) {
			t.Fatalf("ws: AssertNoMatch: forbidden message arrived: %v", msg)
			return false
		}
	}
}

// SendJSON marshals msg to JSON and sends it as a WebSocket text frame.
// Use this to inject client-to-server messages in tests (e.g. PTY input).
func (w *WSWatcher) SendJSON(
	t *testing.T,
	msg any,
) {
	t.Helper()
	raw, err := json.Marshal(msg)
	require.NoError(t, err, "ws: SendJSON marshal")
	require.NoError(t, w.conn.WriteMessage(websocket.TextMessage, raw), "ws: SendJSON write")
}

// ReadRawMsg reads one raw WebSocket frame (binary or text) within timeout.
// Use this for non-JSON protocols such as terminal PTY streams.
func (w *WSWatcher) ReadRawMsg(
	t *testing.T,
	timeout time.Duration,
) []byte {
	t.Helper()
	require.NoError(
		t,
		w.conn.SetReadDeadline(time.Now().Add(timeout)),
		"ws: SetReadDeadline",
	)
	msgType, raw, err := w.conn.ReadMessage()
	require.NoError(
		t,
		err,
		"ws: ReadMessage failed",
	)
	require.True(
		t,
		msgType == websocket.BinaryMessage || msgType == websocket.TextMessage,
		"unexpected ws message type %d, want binary (2) or text (1)",
		msgType,
	)
	return raw
}
