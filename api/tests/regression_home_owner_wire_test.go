//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
