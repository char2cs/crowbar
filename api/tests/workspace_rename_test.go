//go:build integration

package tests

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// runGitOut is runGit's reading counterpart: it returns stdout so a test can
// assert on what git reports rather than only that the command succeeded.
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

// renameDTO is workspaceDTO plus the worktree path, which the rename has to move
// in lockstep with the branch.
type renameDTO struct {
	ID        string `json:"id"`
	Branch    string `json:"branch"`
	LocalPath string `json:"localPath"`
}

func renamedWorkspace(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) renameDTO {
	t.Helper()
	var out renameDTO
	h.get("/v0/projects/"+imported.projectID+"/repos/"+imported.repoID+
		"/workspaces/"+wsID, &out)
	return out
}

// The agent chats tree lives beside the worktree inside the workspace root, so a
// rename that moved only the worktree would orphan a workspace's chat history.
func TestRegression_RenameWorkspaceBranch_CarriesAgentChats(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	chats := filepath.Join(filepath.Dir(before.LocalPath), "chats")
	require.NoError(t, os.MkdirAll(chats, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chats, "c1.json"), []byte("history"), 0o600))

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "feature/x"}, &out)

	after := renamedWorkspace(t, h, imported, childID)
	moved := filepath.Join(filepath.Dir(after.LocalPath), "chats", "c1.json")
	body, err := os.ReadFile(moved)
	require.NoError(t, err, "chat history must travel with the workspace")
	assert.Equal(t, "history", string(body))
}

// A locked workspace is a protected branch with its own managed worktree;
// renaming it would desynchronise it from the provider, so it is refused.
func TestRegression_RenameWorkspaceBranch_RefusesLockedWorkspace(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// imported.workspaceID is the locked managed worktree for "main".
	resp := h.raw(http.MethodPatch, repoBase+"/workspaces/"+imported.workspaceID,
		map[string]string{"branch": "renamed-main"}, http.StatusConflict)
	_ = resp.Body.Close()

	still := renamedWorkspace(t, h, imported, imported.workspaceID)
	assert.Equal(t, "main", still.Branch, "a refused rename must change nothing")
}

// A rename onto a branch another workspace already holds must be refused before
// anything moves, rather than colliding two workspaces onto one directory.
func TestRegression_RenameWorkspaceBranch_RefusesBranchAlreadyHeld(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	firstID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	createChildWorkspace(t, h, repoBase, "taken", imported.workspaceID)

	resp := h.raw(http.MethodPatch, repoBase+"/workspaces/"+firstID,
		map[string]string{"branch": "taken"}, http.StatusConflict)
	_ = resp.Body.Close()

	after := renamedWorkspace(t, h, imported, firstID)
	assert.Equal(t, "testing", after.Branch, "a refused rename must change nothing")
	assert.DirExists(t, after.LocalPath, "the workspace must stay where it was")
}

// dropWorkspacePathsTable removes the workspace_paths table from the daemon's
// live view.db, so the next workspace-record write fails at its very first
// statement.
//
// It is fault injection at the storage layer, and it is the only seam that makes
// the record write fail DETERMINISTICALLY: the aggregate's own refusals (a
// validation error, OCC retries exhausted, a full command queue) all need either
// a concurrent writer or a wedged runtime to provoke, and reproducing them by
// racing would make the test time-dependent. What matters for the contract under
// test is only THAT the record write returns an error after git and the disk have
// both moved — not which of those produced it.
func dropWorkspacePathsTable(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(h.home, "state", "view.db")),
		&gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err, "open the daemon's view.db")
	require.NoError(t, db.Exec("PRAGMA busy_timeout=5000").Error)
	require.NoError(t, db.Exec("DROP TABLE workspace_paths").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// resolved returns p with every symlink resolved, so a path git reports (fully
// resolved) compares equal to the same path as the record derived it.
func resolved(
	t *testing.T,
	p string,
) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(p)
	require.NoError(t, err, "resolve %s", p)
	return out
}

// TestRegression_RenameWorkspaceBranch_RenamesGitAndRecordAndMovesNothing pins
// the bug this endpoint exists to fix, and the shape that replaced it.
//
// The sidebar's inline rename only ever rewrote local UI state, so the branch
// snapped back on the next reload. Wiring it to the raw `git branch -m` endpoint
// would have been just as wrong: that renames the branch and leaves the record
// still carrying the old one.
//
// Both halves must move — and NOTHING ELSE may. The workspace directory is fixed
// at creation and does not track the branch, which is what keeps a live worktree,
// its git registration and every path recorded against it undisturbed.
func TestRegression_RenameWorkspaceBranch_RenamesGitAndRecordAndMovesNothing(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	require.Equal(t, "testing", before.Branch)
	require.DirExists(t, before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "feature/x"}, &out)

	// 1. The record carries the new branch and the SAME path.
	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "feature/x", after.Branch, "the record must carry the new branch")
	assert.Equal(t, before.LocalPath, after.LocalPath,
		"the workspace must not move: its directory is fixed at creation")

	// 2. The tree is untouched and still where it was.
	assert.DirExists(t, before.LocalPath)

	// 3. git agrees, and the worktree is still usable — a registration broken by a
	//    move would fail this.
	assert.Equal(t, "feature/x",
		strings.TrimSpace(runGitOut(t, after.LocalPath, "branch", "--show-current")))
	runGit(t, after.LocalPath, "status", "--porcelain")
}

// TestRegression_RenameWorkspaceBranch_DeleteHitsOnlyItsOwnDirectory pins the
// data-loss chain a name-following rename left open.
//
// The old rename moved the workspace on disk without re-pointing the id→path
// index the delete reactor resolves from, so the stale row still named the
// PRE-rename directory — which a new workspace on the freed branch name then
// occupied. Deleting the renamed workspace destroyed the NEW one's worktree and
// chats, and orphaned its own tree forever.
//
// Nothing moves now, so the index cannot go stale. The freed BRANCH name is
// reusable immediately; the directory it was created under is not, so the new
// workspace is disambiguated onto its own and the two deletes cannot collide.
func TestRegression_RenameWorkspaceBranch_DeleteHitsOnlyItsOwnDirectory(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	renamedID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, renamedID)
	frozenRoot := filepath.Dir(before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+renamedID, map[string]string{"branch": "renamed"}, &out)
	// The reuse below is legal only once "testing" is free, and the
	// duplicate-branch guard answers from a projection that settles
	// asynchronously — race it and the create is refused for a branch nothing
	// holds any more.
	h.QuiesceReactors()
	after := renamedWorkspace(t, h, imported, renamedID)
	require.Equal(t, "renamed", after.Branch)
	require.Equal(t, before.LocalPath, after.LocalPath, "precondition: nothing moved")

	// The branch name is free again; the directory is not, so the new workspace
	// lands beside it rather than on top of it.
	reuseID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	reuse := renamedWorkspace(t, h, imported, reuseID)
	require.Equal(t, "testing", reuse.Branch)
	require.NotEqual(t, frozenRoot, filepath.Dir(reuse.LocalPath),
		"the new workspace must not be given the frozen directory")
	chats := filepath.Join(filepath.Dir(reuse.LocalPath), "chats")
	require.NoError(t, os.MkdirAll(chats, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chats, "c1.json"), []byte("history"), 0o600))

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodDelete, repoBase+"/workspaces/"+renamedID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == renamedID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.DirExists(t, reuse.LocalPath,
		"deleting one workspace must not remove another's worktree")
	assert.FileExists(t, filepath.Join(chats, "c1.json"),
		"nor another's chat history")
	assert.NoDirExists(t, frozenRoot,
		"the renamed workspace's own directory is what the delete must remove")
}
