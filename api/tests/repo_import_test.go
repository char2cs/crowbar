//go:build integration

package tests

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeWorkspaceDTO mirrors the WorkspaceDTO wire fields this suite asserts on
// (the shared fixtures workspaceDTO omits isDefault/localPath).
type homeWorkspaceDTO struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	IsDefault bool   `json:"isDefault"`
	LocalPath string `json:"localPath"`
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
//   - the repo home (the imported folder) is the special default workspace and is
//     DETACHED to HEAD because it was on a protected branch (develop);
//   - every protected branch that exists (develop, master) gets its OWN
//     Crowbar-managed worktree under the crowbar home, locked, NOT the repo folder;
//   - a protected branch that does not exist (main, from the fallback set) is
//     skipped, not stubbed.
func TestRepoImport_ProtectedBranchesGetManagedWorktrees(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "myrepo")
	// Default branch develop is protected (fallback set), so the home must detach.
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

	// The repo home: default, detached (no branch), rooted at the repo folder.
	var home homeWorkspaceDTO
	for _, w := range byBranch {
		if w.IsDefault {
			home = w
		}
	}
	require.True(t, home.IsDefault, "a default (home) workspace must exist")
	assert.Empty(t, home.Branch, "the repo home is detached off the protected default branch")
	assert.NotEqual(t, "locked", home.Status, "the repo home is never locked")
	assert.Equal(t, repoDir, home.LocalPath, "the repo home stays the imported repo folder")

	// develop + master: each its own managed locked worktree, NOT the repo folder.
	for _, branch := range []string{"develop", "master"} {
		ws := byBranch[branch]
		require.NotEmpty(t, ws.ID, "protected branch %q must have a workspace", branch)
		assert.Equal(t, "locked", ws.Status, "%q is a locked workspace", branch)
		assert.False(t, ws.IsDefault, "%q is not the default", branch)
		assert.NotEqual(t, repoDir, ws.LocalPath, "%q gets a managed worktree, not the repo folder", branch)
		assert.True(t, strings.HasPrefix(ws.LocalPath, h.home),
			"%q managed worktree lives under the crowbar home (%s), got %s", branch, h.home, ws.LocalPath)
		// On disk: the managed worktree is a real checkout on that branch.
		assert.DirExists(t, ws.LocalPath, "%q managed worktree dir exists", branch)
		assert.Equal(t, branch, gitCurrentBranch(t, ws.LocalPath),
			"%q managed worktree is checked out on %q", branch, branch)
	}

	// "main" is in the fallback protected set but does not exist → skipped, not stubbed.
	_, hasMain := byBranch["main"]
	assert.False(t, hasMain, "a non-existent fallback-protected branch is not imported")

	// The repo folder itself is now on a detached HEAD (its branch was freed).
	assert.Empty(t, gitCurrentBranch(t, repoDir),
		"the repo home folder is detached to HEAD so develop is free for its managed worktree")
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
// accumulating the latest per branch, until done(seen) is true or the deadline
// elapses. Frames without an id (e.g. control frames) are skipped.
func collectWorkspacesUntil(
	t *testing.T,
	conn *websocket.Conn,
	done func(map[string]homeWorkspaceDTO) bool,
) map[string]homeWorkspaceDTO {
	t.Helper()
	seen := map[string]homeWorkspaceDTO{}
	deadline := time.Now().Add(10 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	for {
		readUntil(t, conn, func(m map[string]any) bool {
			id, _ := m["id"].(string)
			if id == "" {
				return false
			}
			seen[branchKey(m)] = homeWorkspaceDTO{
				ID:        id,
				RepoID:    asString(m["repoId"]),
				Branch:    asString(m["branch"]),
				Status:    asString(m["status"]),
				IsDefault: m["isDefault"] == true,
				LocalPath: asString(m["localPath"]),
			}
			return true
		})
		if done(seen) {
			return seen
		}
		require.True(t, time.Now().Before(deadline), "deadline before all workspaces materialised; saw %v", keys(seen))
	}
}

// branchKey keys a workspace by branch, falling back to the default marker so the
// detached home (empty branch) does not collide with a branch named "".
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

func keys(m map[string]homeWorkspaceDTO) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
