//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatsWorkOnHomeWorkspace proves the agent chat surface is
// reachable under the project-home group (not only under .../workspaces/:wsId),
// so the FE Chats tab works for a home workspace exactly as the spec requires.
func TestRegression_AgentChatsWorkOnHomeWorkspace(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	var listed []agentChatDTO
	h.get(homeBase+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, created.ID, list[0].ID)

	var providers []struct {
		ID string `json:"id"`
	}
	h.get(homeBase+"/chats/providers", &providers)
	require.NotEmpty(t, providers)
}
