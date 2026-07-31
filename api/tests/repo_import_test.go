//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeWorkspaceDTO mirrors the WorkspaceDTO wire fields this suite asserts on
// (the shared fixtures workspaceDTO omits isDefault/localPath).
type homeWorkspaceDTO struct {
	ID         string `json:"id"`
	RepoID     string `json:"repoId"`
	Branch     string `json:"branch"`
	Status     string `json:"status"`
	IsDefault  bool   `json:"isDefault"`
	LocalPath  string `json:"localPath"`
	HeldByPath string `json:"heldByPath"`
}

// gitRepoWithBranches builds a real git repo with one commit, creates each of
// extraBranches off it, then checks out `checkout`. No remote is configured, so
// the provider falls back to its default protected set (main/develop/master).
func gitRepoWithBranches(
	t *testing.T,
	dir string,
	checkout string,
	extraBranches ...string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@t.dev"},
		{"config", "user.name", "t"},
		{"checkout", "-b", "master"},
	} {
		runGit(t, dir, args...)
	}
	require.NoError(t, writeFile(dir, "README.md", "hello\n"))
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	for _, b := range extraBranches {
		runGit(t, dir, "branch", b)
	}
	runGit(t, dir, "checkout", checkout)
}

func gitCurrentBranch(t *testing.T, dir string) string {
	t.Helper()
	out := gitOutput(t, dir, "branch", "--show-current")
	return strings.TrimSpace(out)
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// TestRepoImport_ProtectedBranchesGetManagedWorktrees is the end-to-end contract
// for the Crowbar workspace model with REAL git:
//   - the repo home (the imported folder) is the special default workspace and
//     STAYS on its branch (develop) — it is no longer force-detached (spec §3.4);
//   - the protected branch the home holds (develop) cannot get its own worktree,
//     so it lands as a PLACEHOLDER row: locked, no worktree on disk, HeldByPath ==
//     the repo folder (spec §3.2);
//   - a protected branch that is free (master) gets its OWN Crowbar-managed
//     worktree under the crowbar home, locked, NOT the repo folder;
//   - a protected branch that does not exist (main, from the fallback set) is
//     skipped, not stubbed.
func TestRepoImport_ProtectedBranchesGetManagedWorktrees(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "myrepo")
	// Default branch develop is protected (fallback set); the home stays on it and
	// the branch is surfaced as a placeholder held by the repo folder (spec §3.4).
	gitRepoWithBranches(t, repoDir, "develop", "develop")
	require.Equal(t, "develop", gitCurrentBranch(t, repoDir), "repo starts checked out on develop")

	// 1. Create the project (lightweight Create — no repo discovery).
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	// 2. Add the repo → ImportRepo runs the new adoption.
	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	resp = h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "myrepo", "path": repoDir}, http.StatusAccepted)
	_ = resp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	// 3. Collect the repo's workspaces from the per-repo stream until the two
	//    protected branches have materialised (snapshot + live frames).
	//
	//    Join the import's post-commit reactors FIRST. Provisioning runs
	//    asynchronously off the repo POST above, so a stream dialled here races
	//    it: the frames for the protected branches can be broadcast before this
	//    connection exists, and the snapshot that would otherwise carry them is
	//    built from an independent read model that settles separately. Lose both
	//    and the collector waits on a branch that is never coming — on Linux it
	//    lost every time, and saw only [(default) develop] with master missing.
	h.QuiesceReactors()
	wsConn := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	byBranch := collectWorkspacesUntil(t, wsConn, func(seen map[string]homeWorkspaceDTO) bool {
		_, dev := seen["develop"]
		_, mas := seen["master"]
		return dev && mas
	})

	// The repo home: default, STAYS on its branch (develop), rooted at the repo folder.
	var home homeWorkspaceDTO
	for _, w := range byBranch {
		if w.IsDefault {
			home = w
		}
	}
	require.True(t, home.IsDefault, "a default (home) workspace must exist")
	assert.Equal(t, "develop", home.Branch,
		"the repo home stays on its protected branch — no longer force-detached (spec §3.4)")
	assert.NotEqual(t, "locked", home.Status, "the repo home is never locked")
	assert.Equal(t, repoDir, home.LocalPath, "the repo home stays the imported repo folder")
	assert.Empty(t, home.HeldByPath, "the repo home is never a placeholder")

	// develop is held by the repo home, so it cannot get its own worktree — it lands
	// as a placeholder: locked, no worktree on disk, HeldByPath == the repo folder.
	dev := byBranch["develop"]
	require.NotEmpty(t, dev.ID, "the held protected branch must still appear as a placeholder row")
	assert.Equal(t, "locked", dev.Status, "the develop placeholder is locked")
	assert.False(t, dev.IsDefault, "the develop placeholder is not the default")
	assert.Empty(t, dev.LocalPath, "a placeholder has no managed worktree on disk")
	assert.True(t, samePathResolved(t, repoDir, dev.HeldByPath),
		"the develop placeholder records the repo folder as the branch holder, got %s", dev.HeldByPath)

	// master is free, so it gets its OWN managed locked worktree, NOT the repo folder.
	mas := byBranch["master"]
	require.NotEmpty(t, mas.ID, "the free protected branch must have a managed workspace")
	assert.Equal(t, "locked", mas.Status, "master is a locked workspace")
	assert.False(t, mas.IsDefault, "master is not the default")
	assert.Empty(t, mas.HeldByPath, "a healthy managed worktree carries no holder")
	assert.NotEqual(t, repoDir, mas.LocalPath, "master gets a managed worktree, not the repo folder")
	assert.True(t, strings.HasPrefix(mas.LocalPath, h.home),
		"master managed worktree lives under the crowbar home (%s), got %s", h.home, mas.LocalPath)
	// On disk: the managed worktree is a real checkout on master.
	assert.DirExists(t, mas.LocalPath, "master managed worktree dir exists")
	assert.Equal(t, "master", gitCurrentBranch(t, mas.LocalPath),
		"master managed worktree is checked out on master")

	// "main" is in the fallback protected set but does not exist → skipped, not stubbed.
	_, hasMain := byBranch["main"]
	assert.False(t, hasMain, "a non-existent fallback-protected branch is not imported")

	// The repo folder itself STAYS on develop — Crowbar no longer detaches the
	// user's checkout without consent; develop is surfaced as a placeholder instead.
	assert.Equal(t, "develop", gitCurrentBranch(t, repoDir),
		"the repo home folder stays on develop — no silent force-detach (spec §3.4)")
}

// TestRegression_RepoImport_SameFolderCannotBeAddedToASecondProject pins the
// one-folder-one-project rule.
//
// Nothing stopped a folder already imported under one project from being added
// to another: the dedup was scoped to the target project, so a second Repository
// row appeared over the same .git. The two can never both work — git checks a
// branch out in one worktree at a time, so the second import claims none of the
// protected branches its sibling already holds and lands a placeholder for each
// one. The user gets a repository that looks imported and can manage nothing.
//
// The add is now refused up front, synchronously (the endpoint is 202-async, so
// a refusal raised during the import would reach the client as a timeout), with
// the owning project named, and no row is created.
func TestRegression_RepoImport_SameFolderCannotBeAddedToASecondProject(t *testing.T) {
	h := newHarness(t)

	parentA := t.TempDir()
	repoDir := filepath.Join(parentA, "shared")
	gitRepoWithBranches(t, repoDir, "master", "main")

	projectsWS := h.dial("/v0/projects")
	projectA := createProject(t, h, projectsWS, parentA, "Alpha")
	addRepo(t, h, h.dial("/v0/projects/"+projectA+"/repos"), projectA, repoDir)

	// A second project tries to adopt the same folder.
	projectB := createProject(t, h, projectsWS, t.TempDir(), "Beta")
	resp := h.raw(http.MethodPost, "/v0/projects/"+projectB+"/repos",
		map[string]string{"name": "shared", "path": repoDir}, http.StatusConflict)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "Alpha",
		"the refusal must name the project that already has the folder, got %s", body)

	// Nothing was created under project B. The refusal is SYNCHRONOUS — it returns
	// before any background work is scheduled — so this read needs no barrier: had
	// the add been accepted, the row would already exist by the time the response
	// came back.
	var reposB []struct {
		Path      string `json:"path"`
		ProjectID string `json:"projectId"`
	}
	h.get("/v0/projects/"+projectB+"/repos?projectId="+projectB, &reposB)
	for _, r := range reposB {
		assert.NotEqual(t, repoDir, r.Path,
			"a refused add must leave no repository row behind")
	}
}

// TestRegression_RepoImport_ProtectedBranchHeldByAnOrphanWorktree_StillGetsARow
// pins the fix for an import that produced NO locked workspace and said nothing.
//
// Removing a repository deletes its row but leaves its LOCKED managed worktrees
// on disk, so re-adding the folder meets its own protected branch still checked
// out under the crowbar home. holder.Resolve classifies any holder there as
// HeldByManaged, and the import read that as "already represented by a managed
// workspace — never double-provision" and returned nil. But that judgement was
// made from the FILESYSTEM, not from this repo's rows: the worktree belongs to a
// repo aggregate that no longer exists, so the branch produced no row for the new
// repo and — the skip being the one silent path in provisioning — no warning
// either. The repo imported with nothing but its home: no locked root, and
// nothing to explain why.
//
// Every protected branch must yield a row. A managed holder this repo does not
// own is a live holder like any other, so it lands as a PLACEHOLDER (spec §3.3).
func TestRegression_RepoImport_ProtectedBranchHeldByAnOrphanWorktree_StillGetsARow(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "shared")
	// master is HEAD (so it is the home's branch); main is free for the first
	// import to claim. Both are in the fallback protected set, which is resolved in
	// the fixed order main → develop → master.
	gitRepoWithBranches(t, repoDir, "master", "main")

	projectsWS := h.dial("/v0/projects")
	projectID := createProject(t, h, projectsWS, parent, "Alpha")
	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	firstRepoID := addRepo(t, h, reposWS, projectID, repoDir)

	first := collectWorkspacesUntil(t,
		h.dial("/v0/projects/"+projectID+"/repos/"+firstRepoID+"/workspaces"),
		func(seen map[string]homeWorkspaceDTO) bool {
			_, ok := seen["main"]
			return ok
		})
	orphan := first["main"]
	require.Equal(t, "locked", orphan.Status, "the first import claims main as a locked workspace")
	require.NotEmpty(t, orphan.LocalPath, "the first import gets a real managed worktree for main")

	// Remove the repository. Its locked worktree is deliberately left on disk (the
	// removal guard never touches a locked row), so the branch stays checked out.
	resp := h.raw(http.MethodDelete,
		"/v0/projects/"+projectID+"/repos/"+firstRepoID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == firstRepoID && m["status"] == "deleted"
	})
	require.DirExists(t, orphan.LocalPath,
		"the locked worktree outlives the repo row — that is what makes this reachable")

	// Re-add the folder. Allowed: the row that owned it is gone.
	secondRepoID := addRepo(t, h, reposWS, projectID, repoDir, firstRepoID)

	// Join the import's post-commit reactors, then read a settled snapshot.
	//
	// This used to wait for "master" to arrive as its own row, on the reasoning
	// that master is resolved after main and so proves main's decision landed.
	// That signal is UNREACHABLE: this fixture checks out master, so master is
	// the repo HOME, and branchKey files every default row under "(default)".
	// seen["master"] was only ever set when a frame raced in before isDefault
	// had become true — the test passed only by catching a transient wrong
	// state, and hung forever when it did not. On Linux it never did: 0/5.
	h.QuiesceReactors()

	second := collectWorkspacesUntil(t,
		h.dial("/v0/projects/"+projectID+"/repos/"+secondRepoID+"/workspaces"),
		func(seen map[string]homeWorkspaceDTO) bool {
			_, ok := seen["main"]
			return ok
		})

	mainRow, ok := second["main"]
	require.True(t, ok,
		"a protected branch held by an orphaned managed worktree must still produce a row — "+
			"skipping it imports the repo with no locked workspace and no warning")
	assert.Equal(t, "locked", mainRow.Status, "the row is locked like every protected-branch row")
	assert.False(t, mainRow.IsDefault, "it is not the repo home")
	assert.Empty(t, mainRow.LocalPath, "it is a placeholder — the branch is checked out elsewhere")
	assert.True(t, samePathResolved(t, orphan.LocalPath, mainRow.HeldByPath),
		"the placeholder names the holder so it can explain itself: want %s, got %s",
		orphan.LocalPath, mainRow.HeldByPath)
	assert.NotEqual(t, orphan.ID, mainRow.ID, "each repo aggregate owns its own row")
}

// createProject creates a project rooted at path and returns its id once the
// broadcast lands. projectsWS must be an already-dialled /v0/projects stream so
// no frame is missed.
func createProject(
	t *testing.T,
	h *harness,
	projectsWS *websocket.Conn,
	path string,
	name string,
) string {
	t.Helper()
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": name, "path": path}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == path })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)
	return projectID
}

// addRepo adds repoPath to projectID and returns the new repo's id once its DTO
// lands on the already-dialled reposWS. skip lists repo ids to ignore: a re-add
// of a folder that was imported before sees the old row replayed in the
// snapshot-on-subscribe burst, and matching on path alone would answer with it.
func addRepo(
	t *testing.T,
	h *harness,
	reposWS *websocket.Conn,
	projectID string,
	repoPath string,
	skip ...string,
) string {
	t.Helper()
	skipped := map[string]bool{}
	for _, id := range skip {
		skipped[id] = true
	}
	resp := h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": filepath.Base(repoPath), "path": repoPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		return m["path"] == repoPath && !skipped[id]
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)
	return repoID
}

// TestRepoImport_UnbornBranchRepo_DegradesGracefully proves the git-safety
// degrade: a freshly-initialized repo (no commits, unborn default branch) cannot
// be detached — real `git switch --detach` fails with "branch yet to be born".
// Import must NOT fail; it degrades to importing the repo with its home on the
// unborn branch.
func TestRepoImport_UnbornBranchRepo_DegradesGracefully(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "empty")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runGit(t, repoDir, "init", "-b", "main") // unborn main, zero commits

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	resp = h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "empty", "path": repoDir}, http.StatusAccepted)
	_ = resp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "the repo is still imported despite the unborn branch (degrade, not fail)")

	// The repo has a home (default) workspace and was not rolled back.
	wsConn := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	seen := collectWorkspacesUntil(t, wsConn, func(s map[string]homeWorkspaceDTO) bool {
		_, ok := s["(default)"]
		return ok
	})
	home := seen["(default)"]
	assert.True(t, home.IsDefault, "the unborn repo still gets its home workspace")
	assert.Equal(t, repoDir, home.LocalPath)
}

// TestRegression_RepoImport_HonorsSuppliedName pins the fix for the add-repo bug
// where the user-entered repository name was silently dropped: persistRepo called
// the importer without the POST body's Name, and the importer hard-derived the
// name from filepath.Base(path). Import a repo whose folder base ("widget")
// differs from the supplied name and prove the broadcast RepoDTO carries the
// SUPPLIED name and a matching generated avatar — not the folder base.
func TestRegression_RepoImport_HonorsSuppliedName(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget") // folder base is "widget"
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	resp = h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "My Custom Name", "path": repoDir}, http.StatusAccepted)
	_ = resp.Body.Close()

	repo := readUntil(t, reposWS, func(m map[string]any) bool { return m["path"] == repoDir })
	assert.Equal(t, "My Custom Name", repo["name"],
		"the repo must be named from the POST body, not filepath.Base(path)")
	assert.Equal(t, "M", repo["avatarLabel"],
		"the generated avatar label is derived from the supplied name")
}

// TestRegression_RepoRename_UpdatesNameAndBroadcasts pins the new rename endpoint:
// PATCH /v0/projects/:projectId/repos/:repoId renames an already-imported repo,
// updating the name and its generated avatar, and broadcasts the updated RepoDTO
// on the repos WS stream so every client's sidebar refreshes.
func TestRegression_RepoRename_UpdatesNameAndBroadcasts(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget")
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	resp = h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "widget", "path": repoDir}, http.StatusAccepted)
	_ = resp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	resp = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoID,
		map[string]string{"name": "Renamed Repo"}, http.StatusNoContent)
	_ = resp.Body.Close()

	renamed := readUntil(t, reposWS, func(m map[string]any) bool { return m["name"] == "Renamed Repo" })
	assert.Equal(t, repoID, renamed["id"], "the rename frame is for the same repo")
	assert.Equal(t, "R", renamed["avatarLabel"], "the generated avatar tracks the new name")
}

// collectWorkspacesUntil reads WorkspaceDTO frames off a workspaces WS,
// accumulating the latest per branch, until done(seen) is true. Frames without
// an id (e.g. control frames) are skipped.
//
// It blocks on real frames and carries no deadline: each WorkspaceDTO's arrival
// is the signal that another workspace has materialised. If one never does, the
// read parks in readUntil and `go test -timeout` names the stuck test — instead
// of an "i/o timeout" that says nothing about which workspace was missing.
func collectWorkspacesUntil(
	t *testing.T,
	conn *websocket.Conn,
	done func(map[string]homeWorkspaceDTO) bool,
) map[string]homeWorkspaceDTO {
	t.Helper()
	seen := map[string]homeWorkspaceDTO{}
	// readUntil aborts the test from inside the loop when its bound expires, so
	// report what DID arrive from a cleanup — otherwise the only thing on record
	// is "no matching frame", which never says which branches the import
	// actually produced and which one went missing.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Logf("collectWorkspacesUntil saw %d workspace(s): %v", len(keys), keys)
	})
	for {
		readUntil(t, conn, func(m map[string]any) bool {
			id, _ := m["id"].(string)
			if id == "" {
				return false
			}
			seen[branchKey(m)] = homeWorkspaceDTO{
				ID:         id,
				RepoID:     asString(m["repoId"]),
				Branch:     asString(m["branch"]),
				Status:     asString(m["status"]),
				IsDefault:  m["isDefault"] == true,
				LocalPath:  asString(m["localPath"]),
				HeldByPath: asString(m["heldByPath"]),
			}
			return true
		})
		if done(seen) {
			return seen
		}
	}
}

// branchKey keys a workspace by branch, falling back to the default marker for the
// repo home so it does not collide with the same-branch placeholder row (the home
// now stays on its protected branch instead of detaching, spec §3.4).
func branchKey(m map[string]any) string {
	if m["isDefault"] == true {
		return "(default)"
	}
	return asString(m["branch"])
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// samePathResolved reports whether two filesystem paths point at the same
// location, resolving symlinks first: git worktree list (and thus a placeholder's
// HeldByPath) emits fully-resolved paths (macOS /var -> /private/var) while the
// imported repo path is not resolved, so a naive string compare would miss.
func samePathResolved(t *testing.T, a string, b string) bool {
	t.Helper()
	ra, err := filepath.EvalSymlinks(a)
	require.NoError(t, err)
	rb, err := filepath.EvalSymlinks(b)
	require.NoError(t, err)
	return ra == rb
}

// TestRegression_RepoNameCannotEscapeTheCrowbarHome pins a traversal the repo
// rename endpoint newly opened up. The create endpoint validated only that the
// name was non-empty; a repository with no parseable git remote falls back to
// that name for its on-disk worktree slug, and filepath.Join CLEANS the result —
// so a name of "../../../../tmp/pwned" derived workspace directories OUTSIDE the
// crowbar home, where they can never be reclaimed (every removal guard refuses
// to touch anything that is not strictly under home). The rename endpoint takes
// the same unsanitised name and feeds it to the same fallback, so both are
// guarded.
func TestRegression_RepoNameCannotEscapeTheCrowbarHome(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	escape := "../../../../" + filepath.Base(t.TempDir()) + "/pwned"

	resp := h.raw(http.MethodPost, "/v0/projects/"+imported.projectID+"/repos",
		map[string]string{"name": escape, "path": imported.repoPath},
		http.StatusBadRequest)
	_ = resp.Body.Close()

	resp = h.raw(http.MethodPatch,
		"/v0/projects/"+imported.projectID+"/repos/"+imported.repoID,
		map[string]string{"name": escape}, http.StatusBadRequest)
	_ = resp.Body.Close()

	// The repo kept the name it was imported with: a refused rename changes
	// nothing, so nothing downstream ever derives a path from the escape.
	var repo struct {
		Name string `json:"name"`
	}
	h.get("/v0/projects/"+imported.projectID+"/repos/"+imported.repoID, &repo)
	assert.Equal(t, "demo", repo.Name)
}

// TestRegression_RepoRename_KeepsWorktreesUnderTheOriginalPathSlug pins the fix
// for the on-disk slug tracking the DISPLAY NAME. Repository.Name doubles as the
// worktree slug for a repo with no parseable remote, and this branch made that
// name client-supplied twice over: the add-repo form sets it and the new PATCH
// endpoint rewrites it. Each derivation then read whatever the name currently
// was, so the repo's tree forked in two — new workspaces under the new slug, the
// existing ones stranded under the old — and the sibling scan that rejects a
// case-only path clash read an empty directory and passed unconditionally.
//
// The slug now comes from the repo's PATH, seeded once at import, so a repo
// imported from .../widget keeps deriving under widget/ no matter what the user
// calls it.
func TestRegression_RepoRename_KeepsWorktreesUnderTheOriginalPathSlug(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget") // the on-disk identity
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	// Imported under a display name that has nothing to do with the folder.
	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	resp = h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "My Custom Name", "path": repoDir}, http.StatusAccepted)
	_ = resp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	slugDir := filepath.Join(h.home, "projects", projectID, "widget")
	createWorkspaceOnBranch(t, h, projectID, repoID, "feature/before")
	require.DirExists(t, filepath.Join(slugDir, "feature", "before", "worktree"),
		"the import must derive under the PATH slug, not the supplied display name")

	resp = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoID,
		map[string]string{"name": "Renamed Repo"}, http.StatusNoContent)
	_ = resp.Body.Close()
	readUntil(t, reposWS, func(m map[string]any) bool { return m["name"] == "Renamed Repo" })

	createWorkspaceOnBranch(t, h, projectID, repoID, "feature/after")
	require.DirExists(t, filepath.Join(slugDir, "feature", "after", "worktree"),
		"a workspace created AFTER the rename must join the repo's existing tree")
	require.NoDirExists(t, filepath.Join(h.home, "projects", projectID, "Renamed Repo"),
		"a rename must never open a second tree for the same repo")
	require.DirExists(t, filepath.Join(slugDir, "feature", "before", "worktree"),
		"the pre-rename workspace must not be stranded")
}

// createWorkspaceOnBranch runs the async workspace create and returns once the
// repo-scoped stream has delivered the resulting WorkspaceDTO — the frame's
// arrival is the completion signal, so the worktree is on disk by the time this
// returns.
func createWorkspaceOnBranch(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	base := "/v0/projects/" + projectID + "/repos/" + repoID
	workspacesWS := h.dial(base + "/workspaces")
	resp := h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": branch}, http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == branch && m["status"] == "new"
	})
	id, _ := created["id"].(string)
	require.NotEmpty(t, id, "workspace create must broadcast an id")
	return id
}
