//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"strings"
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
	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches("feature/a"), http.StatusAccepted)
	require.NoError(t, resp.Body.Close())
	created := feed.awaitNewChat(hasWorkspace)
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

// importedRows imports branches off main as one batch and returns each one's
// chat row by branch, once the import has settled.
func importedRows(
	t *testing.T,
	h *harness,
	f importFixture,
	branches ...string,
) map[string]chatRow {
	t.Helper()
	op := startOperation(t, h, f.repoBase()+"/chats/import-batch", importBranches(branches...))
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, op.ID, operationSettled).State)
	h.Quiesce()
	rows := map[string]chatRow{}
	for _, c := range listChats(t, h, f.projectID, f.repoID) {
		if c.Worktree != nil && c.WorkspaceID != "" {
			rows[c.Worktree.Branch] = c
		}
	}
	for _, b := range branches {
		require.Contains(t, rows, b, "import must land a workspace for %s", b)
	}
	return rows
}

// reparentSettled moves child's workspace under newParent's and waits for the move.
func reparentSettled(
	t *testing.T,
	h *harness,
	f importFixture,
	child chatRow,
	newParent chatRow,
) domain.Operation {
	t.Helper()
	path := f.repoBase() + "/chats/" + owningChatID(t, h, child.WorkspaceID) + "/reparent"
	op := startOperation(t, h, path, map[string]any{"newParentId": newParent.WorkspaceID})
	settled := awaitOperation(t, h, op.ID, operationSettled)
	h.Quiesce()
	return settled
}

// TestRegression_ReparentKeepsAnImportedBranchsCommits: an imported branch's
// recorded fork point was its own tip, so the rebase replayed nothing and the
// move reset the branch onto its new parent, dropping its commits.
func TestRegression_ReparentKeepsAnImportedBranchsCommits(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushFileToOrigin(t, "feature/a", "main", "a.txt", "a\n")
	f.pushFileToOrigin(t, "feature/b", "main", "b.txt", "b\n")
	rows := importedRows(t, h, f, "feature/a", "feature/b")

	settled := reparentSettled(t, h, f, rows["feature/b"], rows["feature/a"])
	require.Equal(t, domain.OpSucceeded, settled.State, "a clean move succeeds: %+v", settled.Error)

	path := rows["feature/b"].Worktree.LocalPath
	own := strings.TrimSpace(runGitOut(t, path, "log", "--format=%s", "feature/a..feature/b"))
	require.Equal(t, "on feature/b", own, "the moved branch keeps its own commit")
	runGit(t, path, "merge-base", "--is-ancestor", "feature/a", "feature/b")
	require.FileExists(t, filepath.Join(path, "b.txt"))
}

// TestRegression_ConflictingReparentIsShownAsAConflict: a move whose rebase
// conflicts is aborted and lands "on paper"; the branch must read as
// conflicting, as a conflicting merge-into-parent does. The merge-tree
// prediction alone misses it: the final trees merge cleanly, a replayed commit
// does not.
func TestRegression_ConflictingReparentIsShownAsAConflict(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushFileToOrigin(t, "feature/b", "main", "f.txt", "x\n")
	f.pushFileToOrigin(t, "feature/b", "feature/b", "f.txt", "y\n")
	f.pushFileToOrigin(t, "feature/base", "main", "f.txt", "y\n")
	rows := importedRows(t, h, f, "feature/b", "feature/base")
	before := strings.TrimSpace(runGitOut(t, rows["feature/b"].Worktree.LocalPath, "rev-parse", "HEAD"))

	settled := reparentSettled(t, h, f, rows["feature/b"], rows["feature/base"])
	require.Equal(t, domain.OpSucceeded, settled.State, "%+v", settled.Error)

	var moved *worktreeWire
	for _, c := range listChats(t, h, f.projectID, f.repoID) {
		if c.WorkspaceID == rows["feature/b"].WorkspaceID {
			moved = c.Worktree
		}
	}
	require.NotNil(t, moved)
	require.Equal(t, rows["feature/base"].WorkspaceID, moved.ParentID, "the move lands on paper")
	require.Equal(t, "pr-conflicts", moved.Status, "the user must be told the branch is not integrated")
	after := strings.TrimSpace(runGitOut(t, moved.LocalPath, "rev-parse", "HEAD"))
	require.Equal(t, before, after, "the aborted rebase leaves the branch where it was")
}
