//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminal_CreateStreamKill proves the PTY lifecycle end to end: create a
// session, attach over the co-located terminal WebSocket
// (/v0/chats/:chatId/terminals/:sessionId/ws), write a command, read its echoed
// output through the PTY, then kill the session (202). A terminal session is
// owned by a chat (wsBase — /v0/chats/:chatId), not addressed by the workspace
// it resolves to; the flat prefix's own resolveChatWorktree middleware finds
// this worktree for the PTY's CWD.
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

// readTerminalUntil blocks reading PTY frames until the decoded data field
// contains want, skipping control and non-JSON frames.
//
// It carries no read deadline: the PTY output IS the signal. If the wanted
// output never arrives the read parks here and `go test -timeout` dumps the
// goroutines, naming this test and this read — a far better report than the
// "i/o timeout" a deadline would produce. A read error (the PTY WS closing
// before the output arrived) still returns false, failing the caller's assert.
func readTerminalUntil(
	t *testing.T,
	conn *websocket.Conn,
	want string,
) bool {
	t.Helper()
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
