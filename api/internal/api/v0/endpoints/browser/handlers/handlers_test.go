package handlers_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/browser"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

func dialPeer(t *testing.T, bridge *browserbridge.Bridge) *websocket.Conn {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	browser.Register(r.Group("/v0"), bridge)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v0/browser/peer", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var hello map[string]any
	require.NoError(t, conn.ReadJSON(&hello))
	require.Equal(t, "hello", hello["type"], "the daemon speaks first so a peer can report its tabs")
	return conn
}

func TestPeer_CarriesAnAgentRequestToTheDesktopAndTheAnswerBack(t *testing.T) {
	bridge := browserbridge.New()
	conn := dialPeer(t, bridge)
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type": "tabs", "tabs": []map[string]any{{"id": "t1", "chatId": "chat1"}},
	}))

	type answer struct {
		out json.RawMessage
		err error
	}
	done := make(chan answer, 1)
	waitForTabs(t, bridge, true)
	go func() {
		out, err := bridge.Do(context.Background(), "chat1", "eval", "t1", json.RawMessage(`{"js":"1"}`))
		done <- answer{out, err}
	}()

	var req map[string]any
	require.NoError(t, conn.ReadJSON(&req))
	require.Equal(t, "request", req["type"])
	require.Equal(t, "eval", req["op"])
	require.Equal(t, "chat1", req["chatId"])
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type": "response", "id": req["id"], "ok": true, "result": 42,
	}))

	got := <-done
	require.NoError(t, got.err)
	require.JSONEq(t, `42`, string(got.out))
}

func TestPeer_DisconnectingForgetsItsTabs(t *testing.T) {
	bridge := browserbridge.New()
	conn := dialPeer(t, bridge)
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type": "tabs", "tabs": []map[string]any{{"id": "t1", "chatId": "chat1"}},
	}))
	waitForTabs(t, bridge, true)
	require.NoError(t, conn.Close())
	waitForTabs(t, bridge, false)
}

// waitForTabs waits for the server's read loop to apply (or drop) chat1's tabs.
func waitForTabs(t *testing.T, bridge *browserbridge.Bridge, present bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		return (string(bridge.Tabs("chat1")) != "[]") == present
	}, 5*time.Second, 5*time.Millisecond)
}
