//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegression_ChatStoppedMidTurn_SpunTheWorkspaceForever(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)
	feed := recordSidebar(t, h)

	postHook(t, h, repoBase(imported), "livestub", runnerID, "user_prompt", `{"prompt":"a long-running task"}`)
	h.Quiesce()
	requireRESTWorking(t, h, imported, true)

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitRunnerGone(chatID, runnerID)
	h.Quiesce()

	require.Empty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID,
		"precondition: the stop took the CLI off the chat, so nothing but the teardown can ever close its turn")
	requireRESTWorking(t, h, imported, false)
}
