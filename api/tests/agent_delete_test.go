//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// waitForFrame drains frames until one satisfies match, tolerating any other
// frame in between — a real signal wait, never a sleep/poll, backstopped by
// frameDeadline.
func waitForFrame(
	t *testing.T,
	frames <-chan map[string]any,
	what string,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := frameDeadline(t)
	var seen []map[string]any
	for {
		select {
		case f := <-frames:
			if match(f) {
				return f
			}
			seen = append(seen, f)
		case <-deadline.Done():
			t.Fatalf("timed out waiting for %s; frames seen: %v", what, seen)
			return nil
		}
	}
}

// waitForChatFrame waits for chatID's kind frame; a delete ends the chat's
// live CLI, so runner frames race the chat's own.
func waitForChatFrame(
	t *testing.T,
	frames <-chan map[string]any,
	chatID string,
	kind string,
) map[string]any {
	t.Helper()
	return waitForFrame(t, frames, fmt.Sprintf("chat %s's %q frame", chatID, kind),
		func(f map[string]any) bool { return f["chatId"] == chatID && f["kind"] == kind })
}

// boundTo reports whether frame's chat snapshot shows chatID bound to sessionID.
func boundTo(frame map[string]any, chatID, sessionID string) bool {
	chat, _ := frame["chat"].(map[string]any)
	conversation, _ := chat["conversation"].(map[string]any)
	return frame["chatId"] == chatID && conversation["sessionId"] == sessionID && conversation["state"] == "bound"
}

// waitForConversation waits for a frame showing chatID bound to sessionID.
func waitForConversation(t *testing.T, frames <-chan map[string]any, chatID, sessionID string) {
	t.Helper()
	waitForFrame(t, frames, fmt.Sprintf("chat %s to bind session %s", chatID, sessionID),
		func(f map[string]any) bool { return boundTo(f, chatID, sessionID) })
}

// TestAgentDelete_HardDeletesAndBroadcastsScopedDeleted proves the Task 5
// hard-delete route end-to-end: DELETE .../chats/:id responds 202,
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
	frames := dialAgentWS(t, h, repoBase(ws)+"/chats/ws")

	chatID := createAgentChat(t, h, ws)
	waitForChatFrame(t, frames, chatID, "created")

	resp := h.raw(http.MethodDelete, repoBase(ws)+"/chats/"+chatID, nil, http.StatusAccepted)
	_ = resp.Body.Close()

	deleted := waitForChatFrame(t, frames, chatID, "deleted")
	require.Equal(t, ws.workspaceID, deleted["workspaceId"])

	h.Quiesce()

	var list []agentChatDTO
	h.get(repoBase(ws)+"/chats", &list)
	for _, c := range list {
		require.NotEqual(t, chatID, c.ID, "a purged chat must not appear in List")
	}

	getResp := h.raw(http.MethodGet, repoBase(ws)+"/chats/"+chatID, nil, http.StatusNotFound)
	_ = getResp.Body.Close()
}
