//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A repo's default checkout is LOCKED whenever its branch is protected
// (adoptMainWorktree: Protected = provider says protected, i.e. every
// GitHub-backed import whose home sits on main) or the user locks the header.
// The sidebar never draws it as a row — it is the repo header — but the chat
// writer's repo-root snapshot appends its Node{Kind:workspace} anchor as a
// root sibling (mergeAnchorNode keys on RendersAsBranch alone), so every drop
// on that repo's root counts one invisible row and lands one slot early.
func TestRegression_LockedDefaultCheckoutAnchorIsNotARepoRootSibling(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	// Lock the header (what a protected default branch is from import).
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

	// Drag c1 to the END of the drawn level: rest = [main, c2], index 2.
	h.patch(repoBase(imported)+"/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, nil)
	h.Quiesce()

	after := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn after c1->2: %+v", after)
	assert.Equal(t, []string{imported.workspaceID, c2, c1}, drawnIDs(after), "c1 must land after c2")
}
