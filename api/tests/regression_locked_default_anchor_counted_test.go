//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_LockedDefaultCheckoutAnchorIsNotARepoRootSibling(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	var seeded []struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get(repoBase(imported)+"/workspaces", &seeded)
	var owner string
	for _, w := range seeded {
		if w.ID == defaultWS {
			owner = w.OwningChatID
		}
	}
	require.NotEmpty(t, owner)
	locked := true
	h.raw(http.MethodPost, repoBase(imported)+"/chats/"+owner+"/lock", map[string]any{"locked": &locked}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	c1 := createRootChatIn(t, h, imported, defaultWS)
	c2 := createRootChatIn(t, h, imported, defaultWS)

	before := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn before: %+v", before)
	require.Equal(t, []string{imported.workspaceID, c1, c2}, drawnIDs(before), "main branch row, c1, c2")

	h.patch(repoBase(imported)+"/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, nil)
	h.Quiesce()

	after := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn after c1->2: %+v", after)
	assert.Equal(t, []string{imported.workspaceID, c2, c1}, drawnIDs(after), "c1 must land after c2")
}
