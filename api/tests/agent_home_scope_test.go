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
	// conversationsOnly cannot filter the project home's OWN owning chat out
	// of this list any more (2026-09-08 sidebar-placement-unification Task 9
	// deleted the machinery that retyped it to ChatTypeBranch — its only
	// discriminator from an ordinary conversation, since a home workspace
	// carries no git worktree for conversationsOnly's OTHER check,
	// Worktree.OwningChatID, to key off either). A documented, narrow gap
	// Task 10's own frontend cutover closes — assert the created row is
	// present directly instead of the filtered list's size.
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
