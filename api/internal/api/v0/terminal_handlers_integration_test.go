//go:build integration

package v0_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsReadUntil reads WebSocket frames until pred returns true or timeout elapses.
func wsReadUntil(
	t *testing.T,
	conn *websocket.Conn,
	pred func(data string) bool,
	timeout time.Duration,
) bool {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return false
		}
		var msg struct {
			Data string `json:"data"`
		}
		if jsonErr := json.Unmarshal(raw, &msg); jsonErr != nil {
			continue
		}
		if pred(msg.Data) {
			return true
		}
	}
}

// createSessionWS creates a terminal session via HTTP and dials a real WebSocket for it.
func createSessionWS(
	t *testing.T,
	srv *httptest.Server,
	wsID string,
	r http.Handler,
) (sid string, conn *websocket.Conn) {
	t.Helper()

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/terminals", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	sid = created["sessionId"].(string)

	wsURL := "ws" + srv.URL[len("http"):] + "/v0/ws/terminals/" + sid
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	return sid, conn
}

// TestIntegration_WS_SendReceive exercises the full stack:
// HTTP session create → real WebSocket upgrade → PTY input → PTY output back through WS.
func TestIntegration_WS_SendReceive(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsID := seedWorkspace(t, tc, "ws-full-stack")
	sid, conn := createSessionWS(t, srv, wsID, r)
	t.Cleanup(func() { _ = conn.Close() })

	// Send a keystroke through the WebSocket.
	input, _ := json.Marshal(map[string]string{"data": "echo fullstack\n"})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, input))

	// Read frames until "fullstack" appears in output.
	found := wsReadUntil(t, conn, func(data string) bool {
		return strings.Contains(data, "fullstack")
	}, 5*time.Second)

	assert.True(t, found, "PTY output must arrive through WebSocket")

	// Kill the session cleanly.
	req := httptest.NewRequest(http.MethodDelete, "/v0/terminals/"+sid, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// TestIntegration_WS_Resize sends a resize frame through the WebSocket and
// verifies the session survives and continues producing output.
func TestIntegration_WS_Resize(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsID := seedWorkspace(t, tc, "ws-resize")
	sid, conn := createSessionWS(t, srv, wsID, r)
	t.Cleanup(func() { _ = conn.Close() })

	// Send a resize frame.
	resize, _ := json.Marshal(map[string]any{"type": "resize", "cols": 120, "rows": 40})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, resize))

	// Session should still be alive — send a command and expect output.
	input, _ := json.Marshal(map[string]string{"data": "echo afterresize\n"})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, input))

	found := wsReadUntil(t, conn, func(data string) bool {
		return strings.Contains(data, "afterresize")
	}, 5*time.Second)

	assert.True(t, found, "session must respond after resize")

	req := httptest.NewRequest(http.MethodDelete, "/v0/terminals/"+sid, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// TestIntegration_WS_RingReplay verifies that a second WebSocket client receives
// buffered output produced before it connected.
func TestIntegration_WS_RingReplay(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsID := seedWorkspace(t, tc, "ws-ring")
	sid, conn1 := createSessionWS(t, srv, wsID, r)

	// Produce output through the first connection.
	input, _ := json.Marshal(map[string]string{"data": "echo ringreplay\n"})
	require.NoError(t, conn1.WriteMessage(websocket.TextMessage, input))

	// Wait for output on first connection to confirm it reached the PTY.
	found1 := wsReadUntil(t, conn1, func(data string) bool {
		return strings.Contains(data, "ringreplay")
	}, 5*time.Second)
	require.True(t, found1, "first client must see output before close")
	_ = conn1.Close()

	// Second client connects after the output was produced — ring buffer must replay it.
	wsURL := "ws" + srv.URL[len("http"):] + "/v0/ws/terminals/" + sid
	conn2, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn2.Close() })

	found2 := wsReadUntil(t, conn2, func(data string) bool {
		return strings.Contains(data, "ringreplay")
	}, 5*time.Second)
	assert.True(t, found2, "second client must receive ring-buffered output on connect")

	_ = conn2.Close()
	req := httptest.NewRequest(http.MethodDelete, "/v0/terminals/"+sid, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
