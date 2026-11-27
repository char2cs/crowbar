//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	var found bool
	for _, row := range listed {
		if row.ID == created.ID {
			found = true
		}
	}
	assert.True(t, found, "the created conversation must be in the home chat list")

	var providers []struct {
		ID string `json:"id"`
	}
	h.get(homeBase+"/chats/providers", &providers)
	require.NotEmpty(t, providers)
}
