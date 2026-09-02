//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// terminalChatBase returns the CHAT-scoped terminal route prefix for an
// imported repo's workspace. A terminal session is owned by a chat, so the path
// names the chat that owns the workspace rather than the workspace itself; the
// route's middleware resolves that chat back to this worktree for the PTY's
// CWD.
//
// EnsureOwningChat is the daemon's own live-path mint (the boot backfill
// narrowed to one workspace, same decision by the same code): a workspace
// created mid-run is otherwise owed its owning row only by the NEXT boot.
func terminalChatBase(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	ctx := context.Background()
	ws, err := h.app.Repositories.Workspace.Get(ctx, imported.workspaceID)
	require.NoError(t, err)
	require.NoError(t, h.app.Usecases.AgentChatFolder.EnsureOwningChat(ctx, ws))
	h.Quiesce()

	var dto workspaceDTO
	h.get(wsBase(imported), &dto)
	require.NotEmpty(t, dto.OwningChatID,
		"workspace must carry an owning chat id for the chat-scoped terminal routes")
	return "/v0/chats/" + dto.OwningChatID
}

// TestTerminal_CreateStreamKill proves the PTY lifecycle end to end: create a
// session, attach over the co-located terminal WebSocket
// (/v0/chats/:chatId/terminals/:sessionId/ws), write a command, read its echoed
// output through the PTY, then kill the session (202).
func TestTerminal_CreateStreamKill(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := terminalChatBase(t, h, imported)

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
