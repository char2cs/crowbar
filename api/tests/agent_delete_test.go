//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

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
