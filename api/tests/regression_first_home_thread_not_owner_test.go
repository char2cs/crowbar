//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project home with NO chats and no owner (resolveHome's lazy provision
// mints none; a pre-mint home whose chats were all deleted has none either)
// is never handed to ensureOwner — it only runs per chat row, and GET /home
// resolves over zero rows. So the FIRST thread the user starts there is the
// only candidate the next read sees, the legacy fallback elects it as the
// home's owner, and the sidebar hides it: the pending "New thread" row never
// clears (waitForRootHomeChat) and the conversation is gone from the tree.
func TestRegression_FirstThreadInChatlessHomeIsNotElectedOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	homePath := "/v0/projects/" + p + "/home"

	var home struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get(homePath, &home)
	require.NotEmpty(t, home.OwningChatID)

	// The legacy shape: a home with no chats at all and no owner row.
	ctx := context.Background()
	require.NoError(t, h.app.Usecases.AgentChat.PurgeChat(ctx, home.OwningChatID))
	h.Quiesce()

	var before struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get(homePath, &before)
	assert.NotEqual(t, home.OwningChatID, before.OwningChatID)

	// The space header's Thread button: POST .../home/chats with no parent.
	var thread struct {
		ID string `json:"id"`
	}
	h.post(homePath+"/chats", map[string]string{"provider": "promotestub"}, http.StatusCreated, &thread)
	require.NotEmpty(t, thread.ID)
	h.QuiesceReactors()
	h.Quiesce()

	var rows []agentChatDTO
	h.get(homePath+"/chats", &rows)
	var row *agentChatDTO
	for i := range rows {
		if rows[i].ID == thread.ID {
			row = &rows[i]
		}
	}
	require.NotNil(t, row, "the thread is listed")
	if assert.NotNil(t, row.Worktree, "a home chat carries the home worktree") {
		assert.NotEqual(t, thread.ID, row.Worktree.OwningChatID,
			"the first thread started in a chatless home must not be served as the home's owner")
	}

	var after struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get(homePath, &after)
	assert.NotEqual(t, thread.ID, after.OwningChatID, "GET /home must not name the user's thread as the owner")
	assert.NotEmpty(t, after.OwningChatID, "a chatless home gets a real owner minted on read")
}
