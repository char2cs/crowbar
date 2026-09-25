//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A delete never destroys work that exists nowhere else unless the user saw it
// and said so. Found driving the real app: deleting a repo force-deleted a
// branch Crowbar had made that held an unmerged commit, with uncommitted edits
// in its worktree, behind a dialog that only mentioned worktrees.

type workAtRiskWire struct {
	WorkspaceID      string `json:"workspaceId"`
	Branch           string `json:"branch"`
	UncommittedFiles int    `json:"uncommittedFiles"`
	UnmergedCommits  int    `json:"unmergedCommits"`
}

// deleteResponse issues a DELETE, with the discard consent header when discard.
func deleteResponse(t *testing.T, h *harness, path string, discard bool, wantStatus int) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, h.url+path, nil)
	require.NoError(t, err)
	if discard {
		req.Header.Set("Crowbar-Discard-Work", "true")
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	require.Equal(t, wantStatus, resp.StatusCode, "DELETE %s", path)
	return resp
}

// deleteAccepted issues a DELETE the daemon must accept.
func deleteAccepted(t *testing.T, h *harness, path string, discard bool) {
	t.Helper()
	require.NoError(t, deleteResponse(t, h, path, discard, http.StatusAccepted).Body.Close())
}

// refusedOver sends a DELETE without consent and returns the work it was refused over.
func refusedOver(t *testing.T, h *harness, path string) []workAtRiskWire {
	t.Helper()
	resp := deleteResponse(t, h, path, false, http.StatusConflict)
	defer func() { _ = resp.Body.Close() }()
	var env struct {
		Code string `json:"code"`
		Data struct {
			WorkAtRisk []workAtRiskWire `json:"workAtRisk"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.Equal(t, "work_at_risk", env.Code)
	return env.Data.WorkAtRisk
}

// crowbarBranchWithWork cuts a Crowbar workspace on branch under parentID and
// leaves one commit only it has plus one uncommitted file in its worktree.
func crowbarBranchWithWork(t *testing.T, h *harness, imported importedRepo, branch, parentID string) (string, string) {
	t.Helper()
	wsID, chatID := createWorktree(t, h, imported, branch, parentID)
	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err)
	require.NoError(t, writeFile(ws.WorktreePath, "rounding.go", "package pricing\n"))
	runGit(t, ws.WorktreePath, "add", "rounding.go")
	runGit(t, ws.WorktreePath, "commit", "-m", "round half up")
	require.NoError(t, writeFile(ws.WorktreePath, "notes.txt", "not committed yet\n"))
	return ws.WorktreePath, chatID
}

func TestRegression_DeleteRepo_KeepsUnmergedAndUncommittedWorkUnlessConfirmed(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h) // the home IS the user's main checkout
	worktree, _ := crowbarBranchWithWork(t, h, imported, "feature/pricing-rounding", imported.workspaceID)
	createChildWorkspace(t, h, imported, "feature/merged", imported.workspaceID)
	users := filepath.Join(imported.repoPath, "mine.txt")
	require.NoError(t, os.WriteFile(users, []byte("the user's own untracked file\n"), 0o600))
	repoPath := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	risks := refusedOver(t, h, repoPath)

	require.Len(t, risks, 1, "only the branch with work of its own is at risk")
	assert.Equal(t, "feature/pricing-rounding", risks[0].Branch)
	assert.Equal(t, 1, risks[0].UnmergedCommits)
	assert.Equal(t, 1, risks[0].UncommittedFiles)
	h.QuiesceReactors()
	assert.True(t, branchExists(imported.repoPath, "feature/pricing-rounding"), "the branch is kept")
	assert.True(t, branchExists(imported.repoPath, "feature/merged"), "a refused delete touches nothing")
	assert.FileExists(t, filepath.Join(worktree, "notes.txt"), "the uncommitted edit is kept")
	require.Len(t, listRepos(t, h, imported.projectID), 1, "the repo stays")

	reposWS := h.dial("/v0/projects/" + imported.projectID + "/repos")
	deleteAccepted(t, h, repoPath, true)
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.False(t, branchExists(imported.repoPath, "feature/pricing-rounding"), "confirmed: the branch goes")
	assert.False(t, branchExists(imported.repoPath, "feature/merged"))
	assert.NoDirExists(t, worktree, "confirmed: the worktree goes")
	assert.True(t, branchExists(imported.repoPath, "main"))
	assert.FileExists(t, users, "the main checkout is the user's, and is left as it was")
	assert.DirExists(t, filepath.Join(imported.repoPath, ".git"))
}

// A branch Crowbar created whose commits all exist elsewhere is not at risk:
// the default delete takes it as before.
func TestRegression_DeleteRepo_WithoutConsentStillTakesAMergedCrowbarBranch(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	wsID := createChildWorkspace(t, h, imported, "feature/merged", imported.workspaceID)
	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err)

	reposWS := h.dial("/v0/projects/" + imported.projectID + "/repos")
	deleteAccepted(t, h, "/v0/projects/"+imported.projectID+"/repos/"+imported.repoID, false)
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.False(t, branchExists(imported.repoPath, "feature/merged"))
	assert.NoDirExists(t, ws.WorktreePath)
}

func TestRegression_DeleteWorkspace_KeepsUnmergedAndUncommittedWorkUnlessConfirmed(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	worktree, chatID := crowbarBranchWithWork(t, h, imported, "feature/pricing-rounding", imported.workspaceID)
	chatPath := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID + "/chats/" + chatID

	risks := refusedOver(t, h, chatPath)

	require.Len(t, risks, 1)
	assert.Equal(t, workAtRiskWire{
		WorkspaceID: risks[0].WorkspaceID, Branch: "feature/pricing-rounding",
		UncommittedFiles: 1, UnmergedCommits: 1,
	}, risks[0])
	assert.True(t, branchExists(imported.repoPath, "feature/pricing-rounding"))
	assert.FileExists(t, filepath.Join(worktree, "notes.txt"))

	deleteAccepted(t, h, chatPath, true)
	h.QuiesceReactors()

	assert.False(t, branchExists(imported.repoPath, "feature/pricing-rounding"))
	assert.NoDirExists(t, worktree)
}

// A checkout Crowbar did not create is never handed to `git worktree remove`,
// forced or not — even with consent, and even when it is clean, where git
// itself would not refuse. A linked worktree imported as a repo is adopted in
// place as its home.
func TestRegression_DeleteRepo_NeverRemovesACheckoutCrowbarDidNotCreate(t *testing.T) {
	h := newHarness(t)
	main := gitRepoWithCommit(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, main, "worktree", "add", "-b", "feature/users-own", linked)
	projectID, repoID := createProjectAndRepo(t, h, linked)
	h.Quiesce()

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	deleteAccepted(t, h, "/v0/projects/"+projectID+"/repos/"+repoID, true)
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == repoID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.FileExists(t, filepath.Join(linked, ".git"), "the user's own checkout survives, still a checkout")
	assert.FileExists(t, filepath.Join(linked, "README.md"))
	assert.True(t, branchExists(main, "feature/users-own"))
}
