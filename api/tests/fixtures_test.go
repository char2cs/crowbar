//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// importedRepo bundles the ids a project import yields: the project, its
// discovered repository, and the workspace adopted from the repo's main
// worktree (the on-disk repo path).
type importedRepo struct {
	projectID   string
	repoID      string
	workspaceID string
	repoPath    string
}

// importProject creates a real git repo, imports it as a project over the public
// API, and resolves the adopted workspace. Project import is async (202 + WS,
// spec §4): the project, its discovered repo, and the adopted main-branch
// workspace are each broadcast as DTOs on their hierarchical WS streams. The ids
// are learned from those frames (dial-before-POST), never from a sync body.
func importProject(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == repoPath
	})
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID, "import must broadcast a ProjectDTO with an id")

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID, "import must broadcast a RepoDTO with an id")

	// The adopted main worktree is persisted after the repo in the same
	// background import job, so wait for its WorkspaceDTO on the repo-scoped
	// stream rather than racing a GET against the async adoption.
	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	adopted := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "main"
	})
	wsID, _ := adopted["id"].(string)
	require.NotEmpty(t, wsID, "import must adopt and broadcast a workspace for the repo")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		repoPath:    repoPath,
	}
}

// importWritableWorkspace imports a project and then creates a child workspace
// on a fresh, non-protected branch. The adopted main worktree is locked because
// "main" is a default protected branch (04 §5, 05 §3/§4), so every file/git
// write against it now rejects with 409; write flows must target an unlocked
// workspace. The returned id points at that unlocked child; projectID/repoID
// carry over from the import. Workspace creation is async (202 + WS): the id is
// learned from the WorkspaceDTO{status:"new"} on the repo-scoped stream.
func importWritableWorkspace(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	imported := importProject(t, h)

	wsBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID
	workspacesWS := h.dial(wsBase + "/workspaces")
	resp := h.raw(http.MethodPost, wsBase+"/workspaces",
		map[string]string{"branch": "feature/write"}, http.StatusAccepted)
	_ = resp.Body.Close()

	created := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "feature/write" && m["status"] == "new"
	})
	childID, _ := created["id"].(string)
	require.NotEmpty(t, childID, "child workspace create must broadcast an id")

	imported.workspaceID = childID
	return imported
}

type repoDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Path      string `json:"path"`
}

func listRepos(
	t *testing.T,
	h *harness,
	projectID string,
) []repoDTO {
	t.Helper()
	var repos []repoDTO
	h.get("/v0/projects/"+projectID+"/repos", &repos)
	return repos
}

// workspaceDTO mirrors the migrated WorkspaceDTO wire shape (spec §5): the
// Locked bool is gone (status-based now); ParentID, LastError, CanMergeLocally,
// ParentBranch, and the PR* fields are surfaced.
type workspaceDTO struct {
	ID              string `json:"id"`
	RepoID          string `json:"repoId"`
	ProjectID       string `json:"projectId"`
	Branch          string `json:"branch"`
	ParentID        string `json:"parentId"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	LastError       string `json:"lastError"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
}

func listWorkspaces(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) []workspaceDTO {
	t.Helper()
	var workspaces []workspaceDTO
	h.get("/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces", &workspaces)
	return workspaces
}

// wsBase returns the hierarchical workspace route prefix
// /v0/projects/:p/repos/:r/workspaces/:w for an imported repo's workspace.
func wsBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID +
		"/repos/" + imported.repoID +
		"/workspaces/" + imported.workspaceID
}

func writeFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
