//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/tests/kit"
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

// terminalReadBound is how long a test waits for a PTY to print something.
const terminalReadBound = 30 * time.Second

// readTerminalUntil blocks reading PTY frames until their data contains want,
// skipping control frames. Bounded by terminalReadBound: output that never
// comes fails this test by name instead of hanging the whole package.
func readTerminalUntil(
	t *testing.T,
	conn *websocket.Conn,
	want string,
) bool {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(terminalReadBound)))
	for {
		mt, raw, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("PTY output %q never arrived: %v", want, err)
			return false
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		data, _, ok := kit.ParseTerminalFrame(raw)
		if ok && strings.Contains(string(data), want) {
			return true
		}
	}
}
