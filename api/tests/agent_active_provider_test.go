//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_AgentChatActiveProviderID(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.QuiesceReactors()
	chatID := created.ID

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, chatID, list[0].ID)
	assert.Equal(t, "livestub", list[0].ActiveProviderID)

	var detail struct {
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID, &detail)
	assert.Equal(t, "livestub", detail.ActiveProviderID)
}

func TestRegression_AgentChatActiveProviderID_ProviderThatNeverBoundAConversation(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, runnerA := createStubChat(t, h, imported, "streamstub")
	postHook(t, h, repoBase(imported), "streamstub", runnerA, "session_start", `{"session_id":"sess-a"}`)
	h.Quiesce()

	var switched struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &switched)
	h.QuiesceReactors()

	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitRunnerGone(chatID, switched.ID)

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Empty(t, detail.LiveRunnerID, "the chat must be dormant for this to prove anything")
	assert.Equal(t, []string{"sess-a"}, detail.sessionIDs(),
		"only streamstub ever bound a conversation — quietstub's is the switch marker alone")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"quietstub is who was actually running when the chat went dormant; streamstub's "+
			"conversation is merely older and must not win")

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, "quietstub", list[0].ActiveProviderID)
}

func TestRegression_AgentChatBornOnAProviderThatBindsNothing_StaysThatProviderWhenDormant(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createStubChat(t, h, imported, "quietstub")

	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitRunnerGone(chatID, runnerID)

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID, "the chat must be dormant for this to prove anything")
	require.Empty(t, detail.sessionIDs(), "precondition: quietstub bound no conversation at all")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"a dormant chat that bound nothing still knows what it runs; \"\" is what let the UI guess")

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, "quietstub", list[0].ActiveProviderID)

	var revived struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusOK, &revived)
	h.QuiesceReactors()

	back := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Equal(t, "quietstub", back.ActiveProviderID,
		"reopening a chat revives the vendor it was on — converting it is the bug")
	assert.Empty(t, readInterruptions(t, h, imported, chatID),
		"and a revive is not a switch: it must draw no provider-changed divider")
}
