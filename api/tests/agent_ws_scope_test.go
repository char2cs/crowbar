//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialAgentWS opens the agent-chat lifecycle WebSocket at path (h.dial, which
// registers the connection's Close as test cleanup) and returns a channel fed
// by a background reader goroutine that decodes every text frame. The
// goroutine's only exit conditions are a real signal — a read error (the
// connection closing, e.g. on test cleanup) — never a timer; it is the
// no-sleep/no-poll counterpart to harness_test.go's readUntil, structured as a
// channel so callers can select against the test context's Done() as the sole
// backstop instead of a fixed read deadline.
func dialAgentWS(
	t *testing.T,
	h *harness,
	path string,
) <-chan map[string]any {
	t.Helper()
	conn := h.dial(path)
	frames := make(chan map[string]any, 8)
	go func() {
		defer close(frames)
		for {
			mt, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			var got map[string]any
			if json.Unmarshal(raw, &got) != nil {
				continue
			}
			frames <- got
		}
	}()
	return frames
}

// TestAgentWS_WorkspaceIsolation proves the agent-chat lifecycle WebSocket is
// scoped per workspace (Task 3, agentChatDef's wsId Filter): a subscriber of
// workspace A's feed receives A's own "created" frame and never workspace B's,
// even though B's chat (and its own "created" frame, positively proven on B's
// own connection) was created first.
func TestAgentWS_WorkspaceIsolation(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)

	a := importWritableWorkspace(t, h)
	b := importWritableWorkspace(t, h)

	framesA := dialAgentWS(t, h, wsBase(a)+"/agent/ws/chats")
	framesB := dialAgentWS(t, h, wsBase(b)+"/agent/ws/chats")

	// Create B's chat FIRST and require ITS OWN connection to observe the
	// "created" frame before A's chat is ever created: this is a real,
	// positive proof the event actually fired and reached the WS layer — not
	// an assumption — so that if the wsId filter were broken (matching every
	// workspace), B's already-fired frame would be the very first thing
	// connA's channel delivers below.
	var chatB struct {
		ID string `json:"id"`
	}
	h.post(wsBase(b)+"/agent/chats", map[string]string{"provider": "stub"}, http.StatusCreated, &chatB)
	require.NotEmpty(t, chatB.ID)

	select {
	case f := <-framesB:
		require.Equal(t, "created", f["kind"])
		require.Equal(t, chatB.ID, f["chatId"], "workspace B's own connection must see its own chat's created frame")
	case <-t.Context().Done():
		t.Fatal("timed out waiting for workspace B's own created frame")
	}

	// Now create A's chat. connA's channel is read for the first time here:
	// if isolation were broken, B's frame (already pushed above, chronologically
	// first) would arrive before A's own — so asserting the received frame IS
	// A's is a real proof of isolation, not merely of "A eventually gets its
	// own frame".
	var chatA struct {
		ID string `json:"id"`
	}
	h.post(wsBase(a)+"/agent/chats", map[string]string{"provider": "stub"}, http.StatusCreated, &chatA)
	require.NotEmpty(t, chatA.ID)

	select {
	case f := <-framesA:
		assert.Equal(t, "created", f["kind"])
		assert.Equal(t, chatA.ID, f["chatId"], "workspace-A subscriber must never see workspace B's chat")
		assert.Equal(t, a.workspaceID, f["workspaceId"])
	case <-t.Context().Done():
		t.Fatal("timed out waiting for workspace A's created frame")
	}
}
