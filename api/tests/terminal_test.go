//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

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
	h.del(base+"/terminals/"+session.SessionID, nil, http.StatusOK, &killed)
	assert.Equal(t, session.SessionID, killed.ID)
}

func TestRegression_TerminalResizeKeepsWrappedLines(t *testing.T) {
	h := newHarness(t)
	base := wsBase(importProject(t, h))
	var session struct {
		SessionID string `json:"sessionId"`
	}
	h.post(base+"/terminals", map[string]string{}, http.StatusCreated, &session)
	conn := h.dial(base + "/terminals/" + session.SessionID + "/ws")
	send := func(msg any) {
		raw, err := json.Marshal(msg)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, raw))
	}

	send(map[string]any{"type": "resize", "cols": 80, "rows": 24})
	send(map[string]string{"data": "printf 'https://example.com/%0100d\\n' 0 | tr 0 a; M=RSZ; echo \"${M}_7d1c\"\n"})
	require.True(t, readTerminalUntil(t, conn, "RSZ_7d1c"), "the URL is printed")

	url := "https://example.com/" + strings.Repeat("a", 100)
	for _, cols := range []int{50, 80} {
		send(map[string]any{"type": "resize", "cols": cols, "rows": 24})
		send(map[string]string{"data": fmt.Sprintf("M=R%d; echo \"${M}_ok\"\n", cols)})
		snapshot := readSnapshotUntil(t, conn, fmt.Sprintf("R%d_ok", cols))
		assert.Contains(t, snapshot, url, "the keyframe after the resize to %d columns holds the URL whole", cols)
	}

	h.del(base+"/terminals/"+session.SessionID, nil, http.StatusOK, nil)
}

const terminalReadBound = 30 * time.Second

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

func readSnapshotUntil(
	t *testing.T,
	conn *websocket.Conn,
	want string,
) string {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(terminalReadBound)))
	first := ""
	seen := false
	for {
		mt, raw, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("PTY output %q never arrived: %v", want, err)
			return first
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		data, snapshot, ok := kit.ParseTerminalFrame(raw)
		if !ok {
			continue
		}
		if snapshot && !seen {
			first, seen = string(data), true
		}
		if strings.Contains(string(data), want) {
			return first
		}
	}
}
