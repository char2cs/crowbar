//go:build integration

package tests

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runGitOut(
	t *testing.T,
	dir string,
	args ...string,
) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err, "git %v", args)
	return string(out)
}

func renamedWorkspace(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) workspaceDTO {
	t.Helper()
	return worktreeOf(t, h, imported, wsID)
}

func TestRegression_RenameWorkspaceBranch_CarriesAgentChats(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID, childChatID := createWorktree(t, h, imported, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	chats := filepath.Join(filepath.Dir(before.LocalPath), "chats")
	require.NoError(t, os.MkdirAll(chats, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chats, "c1.json"), []byte("history"), 0o600))

	var out map[string]any
	h.patch(repoBase+"/chats/"+childChatID+"/branch", map[string]string{"branch": "feature/x"}, &out)

	after := renamedWorkspace(t, h, imported, childID)
	moved := filepath.Join(filepath.Dir(after.LocalPath), "chats", "c1.json")
	body, err := os.ReadFile(moved)
	require.NoError(t, err, "chat history must travel with the workspace")
	assert.Equal(t, "history", string(body))
}

func TestRegression_RenameWorkspaceBranch_RefusesLockedWorkspace(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	resp := h.raw(http.MethodPatch, repoBase+"/chats/"+imported.chatID+"/branch",
		map[string]string{"branch": "renamed-main"}, http.StatusConflict)
	_ = resp.Body.Close()

	still := renamedWorkspace(t, h, imported, imported.workspaceID)
	assert.Equal(t, "main", still.Branch, "a refused rename must change nothing")
}

func TestRegression_RenameWorkspaceBranch_RefusesBranchAlreadyHeld(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	firstID, firstChatID := createWorktree(t, h, imported, "testing", imported.workspaceID)
	createChildWorkspace(t, h, imported, "taken", imported.workspaceID)

	resp := h.raw(http.MethodPatch, repoBase+"/chats/"+firstChatID+"/branch",
		map[string]string{"branch": "taken"}, http.StatusConflict)
	_ = resp.Body.Close()

	after := renamedWorkspace(t, h, imported, firstID)
	assert.Equal(t, "testing", after.Branch, "a refused rename must change nothing")
	assert.DirExists(t, after.LocalPath, "the workspace must stay where it was")
}

func TestRegression_RenameWorkspaceBranch_RenamesGitAndRecordAndMovesNothing(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID, childChatID := createWorktree(t, h, imported, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	require.Equal(t, "testing", before.Branch)
	require.DirExists(t, before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/chats/"+childChatID+"/branch", map[string]string{"branch": "feature/x"}, &out)
	h.QuiesceReactors()

	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "feature/x", after.Branch, "the record must carry the new branch")
	assert.Equal(t, before.LocalPath, after.LocalPath,
		"the workspace must not move: its directory is fixed at creation")

	assert.DirExists(t, before.LocalPath)

	assert.Equal(t, "feature/x",
		strings.TrimSpace(runGitOut(t, after.LocalPath, "branch", "--show-current")))
	runGit(t, after.LocalPath, "status", "--porcelain")
}

func TestRegression_RenameWorkspaceBranch_DeleteHitsOnlyItsOwnDirectory(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	renamedID, renamedChatID := createWorktree(t, h, imported, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, renamedID)
	frozenRoot := filepath.Dir(before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/chats/"+renamedChatID+"/branch", map[string]string{"branch": "renamed"}, &out)
	h.QuiesceReactors()
	after := renamedWorkspace(t, h, imported, renamedID)
	require.Equal(t, "renamed", after.Branch)
	require.Equal(t, before.LocalPath, after.LocalPath, "precondition: nothing moved")

	reuseID := createChildWorkspace(t, h, imported, "testing", imported.workspaceID)
	reuse := renamedWorkspace(t, h, imported, reuseID)
	require.Equal(t, "testing", reuse.Branch)
	require.NotEqual(t, frozenRoot, filepath.Dir(reuse.LocalPath),
		"the new workspace must not be given the frozen directory")
	chats := filepath.Join(filepath.Dir(reuse.LocalPath), "chats")
	require.NoError(t, os.MkdirAll(chats, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chats, "c1.json"), []byte("history"), 0o600))

	resp := h.raw(http.MethodDelete, repoBase+"/chats/"+renamedChatID, nil, http.StatusOK)
	_ = resp.Body.Close()
	h.QuiesceReactors()
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		require.NotEqual(t, renamedID, w.ID, "the deleted workspace must be gone from the list")
	}

	assert.DirExists(t, reuse.LocalPath,
		"deleting one workspace must not remove another's worktree")
	assert.FileExists(t, filepath.Join(chats, "c1.json"),
		"nor another's chat history")
	assert.NoDirExists(t, frozenRoot,
		"the renamed workspace's own directory is what the delete must remove")
}
