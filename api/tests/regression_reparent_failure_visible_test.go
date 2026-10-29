//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_ReparentFailureIsVisible: a reparent the daemon cannot complete reports why;
// the client used to infer it from a lastError change and a 30 s timeout.
func TestRegression_ReparentFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushBranchToOrigin(t, "feature/a", "main", "a\n")
	conn := h.dial(f.repoBase() + "/chats/ws")
	resp := h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches("feature/a"), http.StatusAccepted)
	require.NoError(t, resp.Body.Close())
	created := readUntil(t, conn, func(m map[string]any) bool { return m["kind"] == "workspace_set" })
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID)
	chatID := owningChatID(t, h, wsID)

	// Onto itself: guardReparent refuses it after the 202.
	op := startOperation(t, h, f.repoBase()+"/chats/"+chatID+"/reparent", map[string]any{"newParentId": wsID})
	require.Equal(t, domain.OpWorkspaceReparent, op.Kind)
	require.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: wsID}, op.Subject)

	settled := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpFailed, settled.State)
	require.NotNil(t, settled.Error)
	require.Equal(t, "conflict", string(settled.Error.Code))
	require.NotEmpty(t, settled.Error.Message)

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.Equal(t, f.mainWSID, ws.ParentID, "a refused reparent keeps the parent")
}
