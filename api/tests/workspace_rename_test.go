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

// aliasOf finds the navigable <slug>/<branch> symlink the daemon publishes for a
// workspace root. The path is not on the API — it is derived state — so it is
// located the same way a human would: the one link under the project directory
// that points at this root.
func aliasOf(
	t *testing.T,
	h *harness,
	projectID string,
	localPath string,
) string {
	t.Helper()
	root := filepath.Dir(localPath)
	var found string
	require.NoError(t, filepath.WalkDir(filepath.Join(h.home, "projects", projectID),
		func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Type()&os.ModeSymlink == 0 {
				return nil //nolint:nilerr // an unreadable branch is skipped, never fatal
			}
			if target, readErr := os.Readlink(p); readErr == nil && target == root {
				found = p
			}
			return nil
		}))
	require.NotEmpty(t, found, "the workspace must have a published alias")
	return found
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

// TestRegression_RenameWorkspaceBranch_MovesGitRecordAndNameTogether pins the
// bug this endpoint exists to fix: the sidebar's inline branch rename only ever
// rewrote local UI state, so the branch snapped back on the next reload. Wiring
// it to the raw `git branch -m` endpoint would have been just as wrong — that
// renames the branch while leaving the workspace record and the name it is
// filed under pointing at the OLD branch.
//
// What moves has changed. The workspace root is keyed by workspace id now, so a
// rename relocates NOTHING on disk: git takes the new ref, the navigable
// <slug>/<branch> alias is republished, and the record carries the new branch
// against the same path it always had. Renaming a live worktree by moving its
// directory is the thing this layout exists to stop.
func TestRegression_RenameWorkspaceBranch_MovesGitRecordAndNameTogether(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	require.Equal(t, "testing", before.Branch)
	require.DirExists(t, before.LocalPath)
	oldAlias := aliasOf(t, h, imported.projectID, before.LocalPath)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "feature/x"}, &out)

	// 1. The record carries the new branch — at the SAME path.
	after := renamedWorkspace(t, h, imported, childID)
	assert.Equal(t, "feature/x", after.Branch, "the record must carry the new branch")
	assert.Equal(t, before.LocalPath, after.LocalPath,
		"a rename must not move the workspace: the root is keyed by id, not by name")
	assert.DirExists(t, after.LocalPath)

	// 2. The NAME moved: the new alias points at the workspace and the old one is
	//    gone, so nothing is left squatting the branch that was freed.
	newAlias := aliasOf(t, h, imported.projectID, after.LocalPath)
	assert.True(t, strings.HasSuffix(newAlias, filepath.Join("feature", "x")),
		"the alias must be published under the new branch name, got %q", newAlias)
	_, statErr := os.Lstat(oldAlias)
	assert.True(t, os.IsNotExist(statErr), "the previous alias must be withdrawn")

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
// — and a new workspace created on the freed branch name occupied exactly that
// directory — so deleting the renamed workspace deleted the NEW workspace's
// worktree and its sibling chats tree.
//
// Keying the root by workspace id removes the shared directory the chain ran
// through: two workspaces can hold the same branch name at different times and
// never share a root. This asserts the outcome either way — reuse the freed
// name, delete the renamed workspace, and the reused one must be untouched.
func TestRegression_RenameWorkspaceBranch_DeleteDoesNotDestroyTheReusedDirectory(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	renamedID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, renamedID)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+renamedID, map[string]string{"branch": "renamed"}, &out)
	// Join the rename's reactors before anything depends on the branch it FREED.
	// The reuse below is legal only because "testing" is no longer taken, and the
	// duplicate-branch guard (ErrBranchWorkspaceExists → 409) answers from a
	// projection that settles asynchronously: race it and the create is rejected
	// for a branch nothing holds any more.
	h.QuiesceReactors()
	after := renamedWorkspace(t, h, imported, renamedID)
	require.Equal(t, "renamed", after.Branch)

	// A brand-new workspace takes the branch name the rename just freed — and
	// gets its OWN root, which is what makes the old chain unreachable.
	reuseID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	reuse := renamedWorkspace(t, h, imported, reuseID)
	require.NotEqual(t, filepath.Dir(before.LocalPath), filepath.Dir(reuse.LocalPath),
		"reusing a freed branch name must not reuse another workspace's directory")
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
		"deleting the renamed workspace must not remove the reused workspace")
	assert.FileExists(t, filepath.Join(chats, "c1.json"),
		"the reused workspace's chat history must survive another workspace's delete")
	assert.NoDirExists(t, filepath.Dir(after.LocalPath),
		"the renamed workspace's own directory is what the delete must remove")
}

// TestRegression_RenameWorkspaceBranch_FreesTheNestedParentItEmpties pins the
// name-squatting the rename left behind: a nested branch name nests a directory
// deeper, and moving out of it left <slug>/feature standing — empty, invisible
// and permanently blocking a branch actually CALLED "feature", which the clash
// scan then refuses for a directory nothing occupies.
//
// The nesting is in the ALIAS tree now rather than in the worktrees, and the
// squatting is just as possible there, so the same end-to-end proof stands: the
// freed name has to be usable again.
func TestRegression_RenameWorkspaceBranch_FreesTheNestedParentItEmpties(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "feature/x", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	nestedParent := filepath.Dir(aliasOf(t, h, imported.projectID, before.LocalPath))
	require.DirExists(t, nestedParent)

	var out map[string]any
	h.patch(repoBase+"/workspaces/"+childID, map[string]string{"branch": "cleanup"}, &out)

	// require, not assert: a surviving directory blocks the create below outright,
	// so the run must stop here with the real diagnosis rather than park on a
	// broadcast that is never coming.
	require.NoDirExists(t, nestedParent,
		"the emptied nested alias parent must not be left squatting the name")

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
// leave git on the new name while the record keeps the old one — a branch the
// record no longer names, which every later merge and remove fails to find.
//
// The step that can still fail after git is publishing the alias, so that is
// where the fault goes: a FILE where the new alias needs a directory. It used to
// be a file where the moved worktree needed one, which is the same failure at
// the same point of no return — only the work being attempted changed.
func TestRegression_RenameWorkspaceBranch_UnwindsRatherThanSplittingOnFailure(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID := createChildWorkspace(t, h, repoBase, "testing", imported.workspaceID)
	before := renamedWorkspace(t, h, imported, childID)
	slugDir := filepath.Dir(aliasOf(t, h, imported.projectID, before.LocalPath))
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
	// Through the link, not at it: an alias is a symlink, so DirExists (which
	// lstats) would call it a file. Resolving to the worktree is the property.
	assert.DirExists(t, filepath.Join(aliasOf(t, h, imported.projectID, after.LocalPath), "worktree"),
		"the original alias must still resolve to the workspace")
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

// The record write is the LAST step of a rename and the one with no compensation
// of its own: git has already taken the new branch and the alias has already been
// republished by the time it runs, so a failure there must put both back rather
// than leave a branch the record no longer names.
//
// That contract is proven in
// internal/app/usecases/worktree.TestRenameBranch_RecordWriteFailure_WithdrawsTheAliasAndTheBranch,
// which injects the failure at the record boundary directly. It was proven here
// too, by dropping workspace_paths from the live view.db — but a rename no
// longer re-points that index (it moves nothing; Relocate owns the index now),
// so the drop stopped failing the write. The remaining stores are the event log
// and its snapshots, and failing EITHER of those also fails every read the
// assertions need, leaving nothing to observe. A black-box test that cannot
// reach the failure is worse than none.

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
