//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminal_CreateStreamKill proves the PTY lifecycle end to end: create a
// session, attach over the co-located terminal WebSocket
// (.../terminals/:sessionId/ws), write a command, read its echoed output through
// the PTY, then kill the session (202).
func TestTerminal_CreateStreamKill(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := wsBase(imported)

	var session struct {
		SessionID string `json:"sessionId"`
	}
	h.post(
		base+"/terminals",
		map[string]string{},
		http.StatusCreated,
		&session,
	)
	require.NotEmpty(t, session.SessionID)

	conn := h.dial(base + "/terminals/" + session.SessionID + "/ws")

	input, err := json.Marshal(map[string]string{"data": "echo crowbar-e2e\n"})
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, input))

	assert.True(t, readTerminalUntil(t, conn, "crowbar-e2e"), "PTY output must arrive over the WS")

	var killed struct {
		ID string `json:"id"`
	}
	h.del(base+"/terminals/"+session.SessionID, nil, http.StatusAccepted, &killed)
	assert.Equal(t, session.SessionID, killed.ID)
}

// readTerminalUntil loop-reads PTY frames under a deadline until the decoded data
// field contains want. It skips control and non-JSON frames and never sleeps.
func readTerminalUntil(
	t *testing.T,
	conn *websocket.Conn,
	want string,
) bool {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		mt, raw, err := conn.ReadMessage()
		if err != nil {
			return false
		}
		if mt != websocket.TextMessage {
			continue
		}
		var msg struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if strings.Contains(msg.Data, want) {
			return true
		}
	}
}
