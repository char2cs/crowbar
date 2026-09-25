//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The project-home owner rides no repo, and the home chat list used to be
// served with no worktree enrichment at all — so nothing on the wire said
// which home chat owned the home workspace, and the client drew the owner as
// an "Untitled chat" ghost row.
func TestRegression_HomeOwnerCarriesItsWorktreeOnTheWire(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	var home struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+imported.projectID+"/home", &home)
	require.NotEmpty(t, home.OwningChatID)

	var rows []agentChatDTO
	h.get("/v0/projects/"+imported.projectID+"/home/chats", &rows)
	var owner *agentChatDTO
	for i := range rows {
		if rows[i].ID == home.OwningChatID {
			owner = &rows[i]
		}
	}
	require.NotNil(t, owner)
	require.NotNil(t, owner.Worktree, "the home owner's row carries its worktree")
	assert.Equal(t, owner.ID, owner.Worktree.OwningChatID)
	assert.Equal(t, home.ID, owner.WorkspaceID)
}
