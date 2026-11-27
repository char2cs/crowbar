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

type WSWatcher struct {
	conn *websocket.Conn
}

func Dial(
	t *testing.T,
	dialer *websocket.Dialer,
	rawURL string,
) *WSWatcher {
	t.Helper()
	conn, resp, err := dialer.Dial(
		rawURL,
		nil,
	)
	require.NoError(
		t,
		err,
	)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Logf("kit: close websocket: %v", err)
		}
	})
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
		mt, raw, err := w.conn.ReadMessage()
		if err != nil {
			failWSRead(t, err, timeout)
			return nil
		}
		var msg map[string]any
		if mt == websocket.BinaryMessage {
			data, snapshot, ok := ParseTerminalFrame(raw)
			require.True(t, ok, "ws: malformed binary terminal frame")
			msg = map[string]any{"data": string(data), "snapshot": snapshot}
		} else {
			require.NoError(
				t,
				json.Unmarshal(
					raw,
					&msg,
				),
			)
		}
		if match(msg) {
			return msg
		}
	}
}

func (w *WSWatcher) WriteJSON(t *testing.T, v any) {
	t.Helper()
	require.NoError(t, w.conn.WriteJSON(v), "ws: WriteJSON")
}

func (w *WSWatcher) Close(
	t *testing.T,
) {
	t.Helper()
	require.NoError(t, w.conn.Close())
}

func (w *WSWatcher) ReadMsg(
	t *testing.T,
	timeout time.Duration,
) map[string]any {
	t.Helper()
	return w.ReadUntil(t, timeout, func(_ map[string]any) bool { return true })
}

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

func (w *WSWatcher) SendJSON(
	t *testing.T,
	msg any,
) {
	t.Helper()
	raw, err := json.Marshal(msg)
	require.NoError(t, err, "ws: SendJSON marshal")
	require.NoError(t, w.conn.WriteMessage(websocket.TextMessage, raw), "ws: SendJSON write")
}
