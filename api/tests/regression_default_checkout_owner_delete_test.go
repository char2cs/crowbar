//go:build integration

package tests

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_DeletingTheDefaultCheckoutOwnerKeepsTheCheckout(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)
	base := repoBase(imported)

	before := workspaceRow(t, h, imported, imported.workspaceID)
	require.True(t, before.IsDefault)
	require.NotEmpty(t, before.OwningChatID)
	repoPath := before.LocalPath

	unlocked := false
	h.raw(http.MethodPost, base+"/chats/"+before.OwningChatID+"/lock",
		map[string]any{"locked": unlocked}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	resp := h.raw(http.MethodDelete, base+"/chats/"+before.OwningChatID, nil, http.StatusOK)
	resp.Body.Close()
	t.Logf("DELETE owner of the default checkout -> %d", resp.StatusCode)
	h.QuiesceReactors()
	h.Quiesce()

	var rows []workspaceDTO
	h.get(base+"/workspaces", &rows)
	var still *workspaceDTO
	for i := range rows {
		if rows[i].ID == imported.workspaceID {
			still = &rows[i]
		}
	}
	if assert.NotNil(t, still, "the repo's default checkout must survive its owner's delete") {
		assert.NotEqual(t, "deleted", string(still.Status))
	}
	if repoPath != "" {
		_, err := os.Stat(repoPath)
		assert.NoError(t, err, "the main checkout directory must still exist")
	}
}

func TestRegression_DeletingAHomesOnlyChatKeepsTheHome(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"
	var home workspaceDTO
	h.get(homeBase, &home)
	require.NotEmpty(t, home.LocalPath)
	h.Quiesce()
	var chats []agentChatDTO
	h.get(homeBase+"/chats", &chats)
	require.Len(t, chats, 1, "precondition: the home holds exactly one chat")

	resp := h.raw(http.MethodDelete, homeBase+"/chats/"+chats[0].ID, nil, http.StatusOK)
	require.NoError(t, resp.Body.Close())
	h.QuiesceReactors()
	h.Quiesce()

	var after workspaceDTO
	h.get(homeBase, &after)
	assert.Equal(t, home.ID, after.ID, "the project still has its home")
	assert.NotEqual(t, "deleted", after.Status)
	_, err := os.Stat(home.LocalPath)
	assert.NoError(t, err, "the home directory must still exist")
}
