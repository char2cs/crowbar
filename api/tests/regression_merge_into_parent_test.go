//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_MergeStrategyCreatesAMergeCommit: "Create a merge commit"
// fast-forwarded the parent whenever it could, so no merge commit was made.
func TestRegression_MergeStrategyCreatesAMergeCommit(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushFileToOrigin(t, "feature/a", "main", "a.txt", "a\n")
	f.pushFileToOrigin(t, "feature/b", "main", "b.txt", "b\n")
	rows := importedRows(t, h, f, "feature/a", "feature/b")
	require.Equal(t, domain.OpSucceeded, reparentSettled(t, h, f, rows["feature/b"], rows["feature/a"]).State)
	childTip := strings.TrimSpace(runGitOut(t, rows["feature/b"].Worktree.LocalPath, "rev-parse", "HEAD"))

	path := f.repoBase() + "/chats/" + owningChatID(t, h, rows["feature/b"].WorkspaceID) + "/merge-into-parent"
	op := startOperation(t, h, path, map[string]any{"strategy": "merge"})
	settled := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpSucceeded, settled.State, "%+v", settled.Error)

	parents := strings.Fields(runGitOut(t, rows["feature/a"].Worktree.LocalPath, "log", "-1", "--format=%P"))
	require.Len(t, parents, 2, "the parent's tip must be a merge commit")
	require.Equal(t, childTip, parents[1], "the merge commit's second parent is the child's tip")
}

// A merge asked to delete a child whose chat is mid-turn is refused before
// anything merges, with the reason, so nothing is left half-done.
func TestRegression_AMergeThatWouldDeleteAWorkingChildIsRefusedFirst(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	var child chatRow
	c.h.get(repoBase(c.imported)+"/chats/"+c.imported.chatID, &child)
	require.NotNil(t, child.Worktree)
	require.NoError(t, os.WriteFile(filepath.Join(child.Worktree.LocalPath, "child.txt"), []byte("c\n"), 0o600))
	runGit(t, child.Worktree.LocalPath, "add", ".")
	runGit(t, child.Worktree.LocalPath, "commit", "-m", "child")
	parentTip := runGitOut(t, c.imported.repoPath, "rev-parse", "HEAD")
	c.startTurn(t, c.launch, "A")

	resp := c.h.raw(http.MethodPost, repoBase(c.imported)+"/chats/"+c.imported.chatID+"/merge-into-parent",
		map[string]any{"strategy": "merge", "deleteSource": true}, http.StatusConflict)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Contains(t, string(body), "the chat is working: stop it first")
	assert.Equal(t, parentTip, runGitOut(t, c.imported.repoPath, "rev-parse", "HEAD"), "nothing merged")
}
