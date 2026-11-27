//go:build integration

package tests

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type homeWorkspaceDTO struct {
	ID         string `json:"id"`
	RepoID     string `json:"repoId"`
	Branch     string `json:"branch"`
	Status     string `json:"status"`
	IsDefault  bool   `json:"isDefault"`
	LocalPath  string `json:"localPath"`
	HeldByPath string `json:"heldByPath"`
}

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
	return strings.TrimSpace(runGitOut(t, dir, "branch", "--show-current"))
}

func TestRepoImport_ProtectedBranchesGetManagedWorktrees(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "myrepo")
	gitRepoWithBranches(t, repoDir, "develop", "develop")
	require.Equal(t, "develop", gitCurrentBranch(t, repoDir), "repo starts checked out on develop")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "myrepo", "path": repoDir})
	repo := reposWS.read(t, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	h.QuiesceReactors()
	byBranch := collectWorkspacesFromChats(t, h, projectID, repoID)
	_, sawDev := byBranch["develop"]
	_, sawMas := byBranch["master"]
	require.True(t, sawDev, "the held protected branch must have materialised once reactors are drained")
	require.True(t, sawMas, "the free protected branch must have materialised once reactors are drained")

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

	dev := byBranch["develop"]
	require.NotEmpty(t, dev.ID, "the held protected branch must still appear as a placeholder row")
	assert.Equal(t, "locked", dev.Status, "the develop placeholder is locked")
	assert.False(t, dev.IsDefault, "the develop placeholder is not the default")
	assert.Empty(t, dev.LocalPath, "a placeholder has no managed worktree on disk")
	assert.True(t, samePathResolved(t, repoDir, dev.HeldByPath),
		"the develop placeholder records the repo folder as the branch holder, got %s", dev.HeldByPath)

	mas := byBranch["master"]
	require.NotEmpty(t, mas.ID, "the free protected branch must have a managed workspace")
	assert.Equal(t, "locked", mas.Status, "master is a locked workspace")
	assert.False(t, mas.IsDefault, "master is not the default")
	assert.Empty(t, mas.HeldByPath, "a healthy managed worktree carries no holder")
	assert.NotEqual(t, repoDir, mas.LocalPath, "master gets a managed worktree, not the repo folder")
	assert.True(t, strings.HasPrefix(mas.LocalPath, h.home),
		"master managed worktree lives under the crowbar home (%s), got %s", h.home, mas.LocalPath)
	assert.DirExists(t, mas.LocalPath, "master managed worktree dir exists")
	assert.Equal(t, "master", gitCurrentBranch(t, mas.LocalPath),
		"master managed worktree is checked out on master")

	_, hasMain := byBranch["main"]
	assert.False(t, hasMain, "a non-existent fallback-protected branch is not imported")

	assert.Equal(t, "develop", gitCurrentBranch(t, repoDir),
		"the repo home folder stays on develop — no silent force-detach (spec §3.4)")
}

func TestRegression_RepoImport_SameFolderCannotBeAddedToASecondProject(t *testing.T) {
	h := newHarness(t)

	parentA := t.TempDir()
	repoDir := filepath.Join(parentA, "shared")
	gitRepoWithBranches(t, repoDir, "master", "main")

	projectsWS := watchEntities(t, h, stream.KindProject)
	projectA := createProject(t, h, projectsWS, parentA, "Alpha")
	addRepo(t, h, watchEntities(t, h, stream.KindRepo), projectA, repoDir)

	projectB := createProject(t, h, projectsWS, t.TempDir(), "Beta")
	resp := h.raw(http.MethodPost, "/v0/projects/"+projectB+"/repos",
		map[string]string{"name": "shared", "path": repoDir}, http.StatusConflict)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "Alpha",
		"the refusal must name the project that already has the folder, got %s", body)

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

func TestRegression_RepoImport_ProtectedBranchHeldByAnOrphanWorktree_StillGetsARow(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "shared")
	gitRepoWithBranches(t, repoDir, "master", "main")

	projectsWS := watchEntities(t, h, stream.KindProject)
	projectID := createProject(t, h, projectsWS, parent, "Alpha")
	reposWS := watchEntities(t, h, stream.KindRepo)
	firstRepoID := addRepo(t, h, reposWS, projectID, repoDir)

	h.QuiesceReactors()
	first := collectWorkspacesFromChats(t, h, projectID, firstRepoID)
	orphan, ok := first["main"]
	require.True(t, ok, "the first import must have provisioned main once reactors are drained")
	require.Equal(t, "locked", orphan.Status, "the first import claims main as a locked workspace")
	require.NotEmpty(t, orphan.LocalPath, "the first import gets a real managed worktree for main")

	resp := h.raw(http.MethodDelete,
		"/v0/projects/"+projectID+"/repos/"+firstRepoID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	reposWS.read(t, func(m map[string]any) bool {
		return m["id"] == firstRepoID && m["gone"] == true
	})
	waitWorkspaceGone(t, h, orphan.ID)
	holderPath := filepath.Join(h.home, "projects", projectID, "orphan-holder")
	runGit(t, repoDir, "worktree", "add", "--force", holderPath, "main")
	require.DirExists(t, holderPath,
		"a worktree holding the protected branch is what makes this reachable")

	secondRepoID := addRepo(t, h, reposWS, projectID, repoDir, firstRepoID)

	h.QuiesceReactors()

	second := collectWorkspacesFromChats(t, h, projectID, secondRepoID)

	mainRow, ok := second["main"]
	require.True(t, ok,
		"a protected branch held by an orphaned managed worktree must still produce a row — "+
			"skipping it imports the repo with no locked workspace and no warning")
	assert.Equal(t, "locked", mainRow.Status, "the row is locked like every protected-branch row")
	assert.False(t, mainRow.IsDefault, "it is not the repo home")
	assert.Empty(t, mainRow.LocalPath, "it is a placeholder — the branch is checked out elsewhere")
	assert.True(t, samePathResolved(t, holderPath, mainRow.HeldByPath),
		"the placeholder names the holder so it can explain itself: want %s, got %s",
		holderPath, mainRow.HeldByPath)
	assert.NotEqual(t, orphan.ID, mainRow.ID, "each repo aggregate owns its own row")
}

func createProject(
	t *testing.T,
	h *harness,
	projectsWS *entities,
	path string,
	name string,
) string {
	t.Helper()
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": name, "path": path}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == path })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)
	return projectID
}

func addRepo(
	t *testing.T,
	h *harness,
	reposWS *entities,
	projectID string,
	repoPath string,
	skip ...string,
) string {
	t.Helper()
	skipped := map[string]bool{}
	for _, id := range skip {
		skipped[id] = true
	}
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": filepath.Base(repoPath), "path": repoPath})
	repo := reposWS.read(t, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		return m["path"] == repoPath && !skipped[id]
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)
	return repoID
}

func waitWorkspaceGone(
	t *testing.T,
	h *harness,
	wsID string,
) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		h.QuiesceReactors()
		if _, err := h.app.Repositories.Workspace.Get(context.Background(), wsID); err != nil {
			return
		}
		require.True(t, time.Now().Before(deadline),
			"workspace %s was not removed by the repo delete cascade within 45s", wsID)
	}
}

func TestRepoImport_UnbornBranchRepo_DegradesGracefully(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "empty")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runGit(t, repoDir, "init", "-b", "main")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "empty", "path": repoDir})
	repo := reposWS.read(t, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "the repo is still imported despite the unborn branch (degrade, not fail)")

	h.QuiesceReactors()
	seen := collectWorkspacesFromChats(t, h, projectID, repoID)
	home, ok := seen["(default)"]
	require.True(t, ok, "the unborn repo must still get its home workspace")
	assert.True(t, home.IsDefault, "the unborn repo still gets its home workspace")
	assert.Equal(t, repoDir, home.LocalPath)
}

func TestRegression_RepoImport_HonorsSuppliedName(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget")
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "My Custom Name", "path": repoDir})

	repo := reposWS.read(t, func(m map[string]any) bool { return m["path"] == repoDir })
	assert.Equal(t, "My Custom Name", repo["name"],
		"the repo must be named from the POST body, not filepath.Base(path)")
	assert.Equal(t, "M", repo["avatarLabel"],
		"the generated avatar label is derived from the supplied name")
}

func TestRegression_RepoRename_UpdatesNameAndBroadcasts(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget")
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "widget", "path": repoDir})
	repo := reposWS.read(t, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	resp = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoID,
		map[string]string{"name": "Renamed Repo"}, http.StatusNoContent)
	_ = resp.Body.Close()

	renamed := reposWS.read(t, func(m map[string]any) bool { return m["name"] == "Renamed Repo" })
	assert.Equal(t, repoID, renamed["id"], "the rename frame is for the same repo")
	assert.Equal(t, "R", renamed["avatarLabel"], "the generated avatar tracks the new name")
}

func collectWorkspacesFromChats(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) map[string]homeWorkspaceDTO {
	t.Helper()
	seen := map[string]homeWorkspaceDTO{}
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree == nil {
			continue
		}
		w := c.Worktree
		seen[branchKey(w)] = homeWorkspaceDTO{
			ID:         c.WorkspaceID,
			RepoID:     repoID,
			Branch:     w.Branch,
			Status:     w.Status,
			IsDefault:  w.IsDefault,
			LocalPath:  w.LocalPath,
			HeldByPath: w.HeldByPath,
		}
	}
	if t.Failed() {
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Logf("collectWorkspacesFromChats saw %d workspace(s): %v", len(keys), keys)
	}
	return seen
}

func branchKey(w *worktreeWire) string {
	if w.IsDefault {
		return "(default)"
	}
	return w.Branch
}

func samePathResolved(t *testing.T, a, b string) bool {
	t.Helper()
	ra, err := filepath.EvalSymlinks(a)
	require.NoError(t, err)
	rb, err := filepath.EvalSymlinks(b)
	require.NoError(t, err)
	return ra == rb
}

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

	var repo struct {
		Name string `json:"name"`
	}
	h.get("/v0/projects/"+imported.projectID+"/repos/"+imported.repoID, &repo)
	assert.Equal(t, "demo", repo.Name)
}

func TestRegression_RepoRename_KeepsWorktreesUnderTheOriginalPathSlug(t *testing.T) {
	h := newHarness(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "widget")
	gitRepoWithBranches(t, repoDir, "master")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "P", "path": parent}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool { return m["path"] == parent })
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "My Custom Name", "path": repoDir})
	repo := reposWS.read(t, func(m map[string]any) bool { return m["path"] == repoDir })
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	slugDir := filepath.Join(h.home, "projects", projectID, "widget")
	createWorkspaceOnBranch(t, h, projectID, repoID, "feature/before")
	require.DirExists(t, filepath.Join(slugDir, "feature", "before", "worktree"),
		"the import must derive under the PATH slug, not the supplied display name")

	resp = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoID,
		map[string]string{"name": "Renamed Repo"}, http.StatusNoContent)
	_ = resp.Body.Close()
	reposWS.read(t, func(m map[string]any) bool { return m["name"] == "Renamed Repo" })

	createWorkspaceOnBranch(t, h, projectID, repoID, "feature/after")
	require.DirExists(t, filepath.Join(slugDir, "feature", "after", "worktree"),
		"a workspace created AFTER the rename must join the repo's existing tree")
	require.NoDirExists(t, filepath.Join(h.home, "projects", projectID, "Renamed Repo"),
		"a rename must never open a second tree for the same repo")
	require.DirExists(t, filepath.Join(slugDir, "feature", "before", "worktree"),
		"the pre-rename workspace must not be stranded")
}

func createWorkspaceOnBranch(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
	branch string,
) string {
	t.Helper()
	return createChildWorkspace(t, h, importedRepo{projectID: projectID, repoID: repoID}, branch, "")
}
