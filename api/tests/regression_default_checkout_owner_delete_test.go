//go:build integration

package tests

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The repo's own default checkout (IsDefault, WorktreePath == the repo's main
// folder) is served with an owning chat like every other workspace, and every
// worktree verb — delete included — is chat-keyed. Once the checkout is not
// locked (an unprotected default branch, or a user unlock), deleting that
// owner cascades DeleteCascade onto the main checkout: the workspace row is
// dropped and the repo loses its header.
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

	resp := h.raw(http.MethodDelete, base+"/chats/"+before.OwningChatID, nil, http.StatusAccepted)
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
