//go:build integration

package tests

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
