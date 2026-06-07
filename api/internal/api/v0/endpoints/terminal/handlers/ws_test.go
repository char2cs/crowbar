package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func wsRouter(
	eng engineterminal.Engine,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v0/ws/terminals/:sessionId", terminalhandlers.NewWS(eng))
	return r
}

func TestWS_NilEngine503(t *testing.T) {
	r := wsRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/v0/ws/terminals/s1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	env := decodeEnvelope(t, w.Body)
	assert.False(t, env.Success)
	assert.Equal(t, "terminal engine not available", env.Error)
}

func TestWS_SessionNotFound(t *testing.T) {
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)
	r := wsRouter(eng)
	req := httptest.NewRequest(http.MethodGet, "/v0/ws/terminals/ghost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	env := decodeEnvelope(t, w.Body)
	assert.False(t, env.Success)
	assert.Equal(t, "session not found", env.Error)
}

func TestWS_UpgradeFailsForPlainRequest(t *testing.T) {
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	sid, err := eng.Create(context.Background(), "w1", t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Kill(context.Background(), sid) })

	r := wsRouter(eng)
	// A plain GET (no WebSocket upgrade headers) to an existing session must
	// fail the upgrade with 400 rather than streaming.
	req := httptest.NewRequest(http.MethodGet, "/v0/ws/terminals/"+sid, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWS_ConnectAndStream(t *testing.T) {
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	sid, err := eng.Create(context.Background(), "w1", t.TempDir(), nil)
	require.NoError(t, err)

	r := wsRouter(eng)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):] + "/v0/ws/terminals/" + sid
	conn, resp, dialErr := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, dialErr)
	t.Cleanup(func() { _ = conn.Close() })

	// Drive the read pump: a keystroke should echo back through the PTY.
	input, _ := json.Marshal(map[string]string{"data": "echo wsstream\n"})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, input))

	found := wsReadUntil(t, conn, func(data string) bool {
		return strings.Contains(data, "wsstream")
	}, 5*time.Second)
	assert.True(t, found, "PTY output must arrive through the WebSocket")

	require.NoError(t, eng.Kill(context.Background(), sid))
}

func TestWS_ResizeFrame(t *testing.T) {
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	sid, err := eng.Create(context.Background(), "w1", t.TempDir(), nil)
	require.NoError(t, err)

	r := wsRouter(eng)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):] + "/v0/ws/terminals/" + sid
	conn, resp, dialErr := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, dialErr)
	t.Cleanup(func() { _ = conn.Close() })

	resize, _ := json.Marshal(map[string]any{"type": "resize", "cols": 120, "rows": 40})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, resize))

	input, _ := json.Marshal(map[string]string{"data": "echo afterresize\n"})
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, input))

	found := wsReadUntil(t, conn, func(data string) bool {
		return strings.Contains(data, "afterresize")
	}, 5*time.Second)
	assert.True(t, found, "session must respond after resize")

	require.NoError(t, eng.Kill(context.Background(), sid))
}

// wsReadUntil reads frames until pred matches the decoded data field or the
// deadline elapses, returning whether a match arrived.
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
