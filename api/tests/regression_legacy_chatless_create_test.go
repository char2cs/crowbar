//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// A workspace written before the Node forest and the chat-first mint existed
// (every pre-existing production workspace, PR #181's own subject) has neither
// an owning chat nor a Node row. The sidebar ids such a row by its WORKSPACE
// id and posts that as parentId for both Thread and Fork.
func legacyChatlessWorkspace(t *testing.T, h *harness) importedRepo {
	t.Helper()
	imported := importProject(t, h)
	ctx := context.Background()
	require.NoError(t, h.app.Usecases.AgentChat.PurgeChat(ctx, imported.chatID))
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, imported.workspaceID))
	h.Quiesce()

	// Asserted through the usecase, not GET .../workspaces: that read now
	// mints an owner for a chatless row (TestRegression_ChatlessWorkspaceIs
	// ServedWithAnOwner), and these cases pin the create shape a client that
	// still names the WORKSPACE id as parent sends.
	chats, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, imported.workspaceID)
	require.NoError(t, err)
	require.Empty(t, chats, "the fixture must be a chatless row, as production has")
	return imported
}

func TestRegression_ThreadOnALegacyChatlessWorkspaceRow(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := legacyChatlessWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]any{"provider": "promotestub", "parentId": imported.workspaceID, "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
}

func TestRegression_ForkOnALegacyChatlessWorkspaceRow(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := legacyChatlessWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]any{"provider": "promotestub", "parentId": imported.workspaceID, "ownWorktree": true, "branch": "test/test"},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
}
