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

// TestRegression_RenameWorkspaceBranch_MovesGitRecordAndDiskTogether pins the
// bug this endpoint exists to fix: the sidebar's inline branch rename only ever
// rewrote local UI state, so the branch snapped back on the next reload. Wiring
// it to the raw `git branch -m` endpoint would have been just as wrong — that
// renames the branch while leaving the workspace record and its on-disk
// directory named after the OLD branch, so the record disagrees with git and the
// stale directory squats the name forever.
//
// Red against either of those: this asserts all three move together.
func TestRegression_RenameWorkspaceBranch_MovesGitRecordAndDiskTogether(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	require.Equal(t, "testing", before.Branch)
	require.DirExists(t, before.LocalPath)
	oldRoot := filepath.Dir(before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "feature/x"}, &out)

	// 1. The record followed the rename — branch AND path.
	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "feature/x", after.Branch, "the record must carry the new branch")
	assert.NotEqual(t, before.LocalPath, after.LocalPath,
		"the record must point at the relocated worktree")
	assert.True(t, strings.HasSuffix(after.LocalPath, filepath.Join("feature", "x", "worktree")),
		"the worktree must live under the new branch's path, got %q", after.LocalPath)

	// 2. The filesystem followed: the tree is at the new path and the old
	//    directory is not left squatting the old branch's name.
	assert.DirExists(t, after.LocalPath)
	assert.NoDirExists(t, oldRoot, "the old workspace root must not be left behind")

	// 3. git agrees: the worktree is checked out on the renamed branch and is
	//    usable there (a broken registration would fail this).
	assert.Equal(t, "feature/x",
		strings.TrimSpace(runGitOut(t, after.LocalPath, "branch", "--show-current")))
	runGit(t, after.LocalPath, "status", "--porcelain")
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

// TestRegression_RenameWorkspaceBranch_DeleteDoesNotDestroyTheReusedDirectory
// pins the data-loss chain the rename left open: the rename moved the workspace
// on disk but never re-pointed the id→path index the delete reactor resolves the
// directory it rm -rf's from. The stale row still named the PRE-rename directory
// — and a new workspace created on the freed branch name occupies exactly that
// directory — so deleting the renamed workspace deleted the NEW workspace's
// worktree and its sibling chats tree, while the renamed workspace's real
// directory was orphaned forever.
func TestRegression_RenameWorkspaceBranch_DeleteDoesNotDestroyTheReusedDirectory(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	renamedID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, renamedID)
	freedRoot := filepath.Dir(before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+renamedID, map[string]string{"branch": "renamed"}, &out)
	after := renamedWorkspace(t, h, imported, renamedID)
	require.Equal(t, "renamed", after.Branch)

	// A brand-new workspace takes the branch name — and therefore the directory —
	// the rename just freed.
	reuseID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	reuse := renamedWorkspace(t, h, imported, reuseID)
	require.Equal(t, freedRoot, filepath.Dir(reuse.LocalPath),
		"precondition: the new workspace must occupy the directory the rename freed")
	chats := filepath.Join(freedRoot, "chats")
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
		"deleting the renamed workspace must not remove the reused directory")
	assert.FileExists(t, filepath.Join(chats, "c1.json"),
		"the reused workspace's chat history must survive another workspace's delete")
	assert.NoDirExists(t, filepath.Dir(after.LocalPath),
		"the renamed workspace's own directory is what the delete must remove")
}

// TestRegression_RenameWorkspaceBranch_FreesTheNestedParentItEmpties pins the
// name-squatting the rename left behind: a nested branch name nests its
// workspace root a directory deeper, and moving the root out left <slug>/feature
// standing — empty, invisible and permanently blocking a branch actually CALLED
// "feature", which the clash scan and the destination stat then refuse for a
// directory nothing occupies.
func TestRegression_RenameWorkspaceBranch_FreesTheNestedParentItEmpties(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "feature/x", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	nestedParent := filepath.Dir(filepath.Dir(before.LocalPath))
	require.DirExists(t, nestedParent)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "cleanup"}, &out)

	// require, not assert: a surviving directory blocks the create below outright,
	// so the run must stop here with the real diagnosis rather than park on a
	// broadcast that is never coming.
	require.NoDirExists(t, nestedParent,
		"the emptied nested parent must not be left squatting the name")

	// The freed name is genuinely usable again: a new workspace on branch
	// "feature" now provisions where the stale directory used to block it.
	reuseID := createChildWorkspace(t, h, repoBase, "feature", imported.workspaceID)
	reuse := renamedWorkspace(t, h, imported, reuseID)
	assert.Equal(t, "feature", reuse.Branch)
	assert.DirExists(t, reuse.LocalPath,
		"a workspace on the freed nested name must provision a real worktree")
}

// TestRegression_RenameWorkspaceBranch_UnwindsRatherThanSplittingOnFailure is the
// end-to-end counterpart of the client-disconnect unit proof: once `git branch -m`
// has landed, a rename that cannot finish must put the branch BACK rather than
// leave git on the new name while the record and the directory keep the old one —
// a branch the record no longer names, which every later merge and remove fails to
// find. Here the destination's parent is a FILE, so the move cannot happen and the
// compensation is the only thing between this and a split workspace.
func TestRegression_RenameWorkspaceBranch_UnwindsRatherThanSplittingOnFailure(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	slugDir := filepath.Dir(filepath.Dir(before.LocalPath))
	require.NoError(t, os.WriteFile(filepath.Join(slugDir, "blocked"), []byte("not a dir"), 0o600))

	resp := h.raw(http.MethodPatch, repoBase+"/workspaces/"+childID,
		map[string]string{"branch": "blocked/x"}, http.StatusInternalServerError)
	_ = resp.Body.Close()

	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "testing", after.Branch, "the record must still name the original branch")
	assert.DirExists(t, after.LocalPath, "the workspace must stay where it was")
	assert.Equal(t, "testing",
		strings.TrimSpace(runGitOut(t, after.LocalPath, "branch", "--show-current")),
		"git must be put back on the original branch, not left split from the record")
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

// TestRegression_RenameWorkspaceBranch_UnwindsWhenTheRecordWriteFails covers the
// LAST step of a rename, the one that had no compensation. git has landed the new
// branch name and the whole workspace root has already moved by the time the
// record write runs; a failure there returned straight to the caller, leaving git
// and the disk on the new branch with the record still naming the old one. That is
// the same split state the detached-context fix addressed, reached from the other
// end — and every later merge or remove then looks for a branch that is gone.
func TestRegression_RenameWorkspaceBranch_UnwindsWhenTheRecordWriteFails(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	require.Equal(t, "testing", before.Branch)
	slugDir := filepath.Dir(filepath.Dir(before.LocalPath))

	dropWorkspacePathsTable(t, h)

	resp := h.raw(http.MethodPatch, repoBase+"/workspaces/"+childID,
		map[string]string{"branch": "feature/x"}, http.StatusInternalServerError)
	_ = resp.Body.Close()

	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "testing", after.Branch, "the record must still name the original branch")
	assert.Equal(t, before.LocalPath, after.LocalPath, "the record must still point at the old path")
	assert.DirExists(t, after.LocalPath, "the workspace must be back where the record points")
	assert.Equal(t, "testing",
		strings.TrimSpace(runGitOut(t, after.LocalPath, "branch", "--show-current")),
		"git must be put back on the original branch, not left split from the record")
	assert.NoDirExists(t, filepath.Join(slugDir, "feature"),
		"nothing may be left squatting at the abandoned destination")
	// git's registration was repaired onto the new path before the record write
	// failed; the unwind must have pointed it back, so git can still resolve the
	// worktree it is standing in. Both sides are symlink-resolved because git
	// reports the fully-resolved path (macOS /var -> /private/var) while the
	// record carries the path as derived.
	assert.Equal(t, resolved(t, after.LocalPath),
		resolved(t, strings.TrimSpace(runGitOut(t, after.LocalPath, "rev-parse", "--show-toplevel"))),
		"git must resolve the restored worktree at the path the record names")
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
