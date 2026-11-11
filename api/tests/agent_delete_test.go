//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// TestAgentDelete_HardDeletesAndPublishesGone proves the hard-delete route
// end-to-end: the chat is genuinely gone (not merely tombstoned) from both List
// and a direct GET-by-id, and the projects scope carries its gone row — the
// live signal every client relies on to drop the chat without a refetch.
func TestAgentDelete_HardDeletesAndPublishesGone(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	ws := importWritableWorkspace(t, h)

	feed := recordSidebar(t, h)

	chatID := createAgentChat(t, h, ws)
	require.Equal(t, ws.workspaceID, feed.awaitListed(chatID)["workspaceId"])

	resp := h.raw(http.MethodDelete, repoBase(ws)+"/chats/"+chatID, nil, http.StatusOK)
	_ = resp.Body.Close()

	feed.awaitGone(stream.KindChatEntity, chatID)

	h.Quiesce()

	var list []agentChatDTO
	h.get(repoBase(ws)+"/chats", &list)
	for _, c := range list {
		require.NotEqual(t, chatID, c.ID, "a purged chat must not appear in List")
	}

	getResp := h.raw(http.MethodGet, repoBase(ws)+"/chats/"+chatID, nil, http.StatusNotFound)
	_ = getResp.Body.Close()
}
