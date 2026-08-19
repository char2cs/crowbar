//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// waitForChatFrame drains frames until one matching (chatID, kind) arrives,
// tolerating any other frame in between (a delete displaces and kills the chat's
// live CLI, so runner frames race the chat's own) — a real signal wait, never a
// sleep/poll, backstopped only by the test context's Done().
func waitForChatFrame(
	t *testing.T,
	frames <-chan map[string]any,
	chatID string,
	kind string,
) map[string]any {
	t.Helper()
	for {
		select {
		case f := <-frames:
			if f["chatId"] == chatID && f["kind"] == kind {
				return f
			}
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for chat %s's %q frame", chatID, kind)
			return nil
		}
	}
}

// TestAgentDelete_HardDeletesAndBroadcastsScopedDeleted proves the Task 5
// hard-delete route end-to-end: DELETE .../agent/chats/:id responds 202,
// the chat is genuinely gone (not merely tombstoned) from both List and a
// direct GET-by-id, and a subscriber of the chat's own workspace WS feed
// receives a scoped "deleted" frame for it — the live signal every workspace
// client relies on to drop the chat without a refetch.
func TestAgentDelete_HardDeletesAndBroadcastsScopedDeleted(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	ws := importWritableWorkspace(t, h)

	// Dial BEFORE creating the chat so the "created" frame (drained below to
	// make the later "deleted" read unambiguous) is never missed.
	frames := dialAgentWS(t, h, wsBase(ws)+"/agent/ws/chats")

	chatID := createAgentChat(t, h, ws)
	waitForChatFrame(t, frames, chatID, "created")

	resp := h.raw(http.MethodDelete, wsBase(ws)+"/agent/chats/"+chatID, nil, http.StatusAccepted)
	_ = resp.Body.Close()

	deleted := waitForChatFrame(t, frames, chatID, "deleted")
	require.Equal(t, ws.workspaceID, deleted["workspaceId"])

	h.Quiesce()

	var list []agentChatDTO
	h.get(wsBase(ws)+"/agent/chats", &list)
	for _, c := range list {
		require.NotEqual(t, chatID, c.ID, "a purged chat must not appear in List")
	}

	getResp := h.raw(http.MethodGet, wsBase(ws)+"/agent/chats/"+chatID, nil, http.StatusNotFound)
	_ = getResp.Body.Close()
}
