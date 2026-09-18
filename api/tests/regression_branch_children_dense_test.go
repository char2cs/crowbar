//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveAChatPlacement(t *testing.T, h *harness, imported importedRepo, chatID string) (string, int) {
	t.Helper()
	detail := getAgentChat(t, h, repoBase(imported), chatID)
	return detail.ParentID, detail.Order
}

// A fork created under a branch row is placed in the workspace-less scope
// (createOwnWorktreeChat -> placeChat(ctx, "", ...)), whose forest walk never
// reaches the destination container, so it lands at order 0 beside a thread
// already at 0; and because the fork's row lives on the Chat aggregate, the
// next thread's Node walk under the same owner never counts it either. The
// level a user builds with the row's own "+" buttons is tied, not dense.
func TestRegression_ThreadAndForkUnderBranchRowAreDense(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	thread := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, imported.chatID)
	var fork struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats", map[string]any{
		"provider": "promotestub", "parentId": imported.chatID, "ownWorktree": true, "branch": "feat-x",
	}, http.StatusCreated, &fork)
	require.NotEmpty(t, fork.ID)
	thread2 := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, imported.chatID)
	h.QuiesceReactors()
	h.Quiesce()

	orders := map[int]string{}
	for _, id := range []string{thread, fork.ID, thread2} {
		parent, order := liveAChatPlacement(t, h, imported, id)
		assert.Equal(t, imported.chatID, parent)
		assert.Emptyf(t, orders[order], "order %d held by both %s and %s", order, orders[order], id)
		orders[order] = id
	}
	assert.Equal(t, thread, orders[0])
	assert.Equal(t, fork.ID, orders[1])
	assert.Equal(t, thread2, orders[2])
}
