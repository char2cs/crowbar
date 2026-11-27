//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_AgentHomeCallbacksReachDaemon(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	feed := recordSidebar(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	feed.awaitListed(created.ID)

	startChat(t, h, homeBase, created.ID)
	detail := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	postHook(t, h, homeBase, "livestub", segID, "session_start", `{"session_id":"sess-home-1"}`)
	feed.awaitConversation(created.ID, "sess-home-1")

	postHook(t, h, homeBase, "livestub", segID, "user_prompt", `{"prompt":"hi"}`)
	feed.awaitTurnStart(created.ID)

	postHook(t, h, homeBase, "livestub", segID, "turn_stop", `{"last_assistant_message":"done"}`)
	feed.awaitTurnEnd(created.ID)

	resp := h.raw(http.MethodPost, homeBase+"/chats/"+created.ID+"/rename?source=agent",
		map[string]string{"title": "Derived home title"}, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitChat(created.ID, "titled", func(chat map[string]any) bool { return chat["title"] == "Derived home title" })
}

func TestRegression_AHomeHookFromAnUnknownRunnerIsRefused(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"
	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	l := stubLaunch(t, h, "livestub", startChat(t, h, homeBase, created.ID))
	require.NoError(t, h.raw(http.MethodDelete, homeBase+"/chats/"+created.ID, nil, http.StatusOK).Body.Close())
	h.QuiesceReactors()
	before := chatDeadLetters(t, h)

	resp := l.PostHook(t, h.url+homeBase, "livestub", "user_prompt", []byte(`{"prompt":"hi"}`)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, before, chatDeadLetters(t, h))
}
