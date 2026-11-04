//go:build integration

package tests

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// pushFixture is an imported branch with a commit to push whose origin is gone.
type pushFixture struct {
	chatID string
	wsID   string
	origin string
}

func failingPush(
	t *testing.T,
	h *harness,
) pushFixture {
	t.Helper()
	f := newImportFixture(t, h)
	f.pushBranchToOrigin(t, "feature/a", "main", "a\n")
	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches("feature/a"), http.StatusAccepted)
	require.NoError(t, resp.Body.Close())
	created := feed.awaitNewChat(hasWorkspace)
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID)

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.WorktreePath)
	require.NoError(t, writeFile(ws.WorktreePath, "b.txt", "b\n"))
	runGit(t, ws.WorktreePath, "add", "b.txt")
	runGit(t, ws.WorktreePath, "commit", "-m", "b")
	require.NoError(t, os.Rename(f.origin, f.origin+".gone"))
	return pushFixture{chatID: owningChatID(t, h, wsID), wsID: wsID, origin: f.origin}
}

// TestRegression_GitPushFailureIsVisible: a push that fails in the background reaches the
// client with its reason instead of only the daemon log.
func TestRegression_GitPushFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)

	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	require.Equal(t, domain.OpGitPush, op.Kind)
	require.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: push.wsID}, op.Subject)

	settled := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpFailed, settled.State)
	require.NotNil(t, settled.Error)
	require.Contains(t, settled.Error.Message, push.origin, "the reason names the remote the push could not reach")
}
