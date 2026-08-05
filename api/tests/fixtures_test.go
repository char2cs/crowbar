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

// importProject creates a real git repo and brings it into Crowbar via the
// two-step public flow: POST /v0/projects creates the project (+ its home
// workspace) and POST /v0/projects/:p/repos adds the repo (ImportRepo).
//
// Under the protected-branch provisioning model the home is NO LONGER
// force-detached: left on "main" it would stay the home AND surface "main" as a
// worktree-less placeholder, yielding two branch=="main" rows. To hand these
// tests a real LOCKED MANAGED "main" worktree unambiguously, the fixture detaches
// the repo's HEAD BEFORE import. The home is then adopted detached (branch==""),
// "main" is free, and ImportRepo provisions it as its own managed locked worktree
// under the crowbar home. The returned workspaceID is that managed locked "main"
// worktree (branch=="main", status "locked", real on-disk worktree at
// <home>/projects/<P>/<R>/workspaces/<W>/worktree); write-path tests use
// importWritableWorkspace to get an unlocked child forked off it. Tests that need
// the home to STAY on its protected branch (and thus a placeholder) use
// importProjectHomeHoldsDefault instead. All ids are learned from the
// hierarchical WS streams (dial-before-POST).
func importProject(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)
	// Detach the home off "main" so the protected default branch is free and
	// provisions as its own managed locked worktree — the workspace these tests
	// operate on. (importProjectHomeHoldsDefault skips this to keep the home on
	// "main" and exercise the placeholder path instead.)
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	// The locked managed worktree for the protected default branch ("main") is
	// persisted during the background ImportRepo job; wait for its WorkspaceDTO on
	// the repo-scoped stream rather than racing a GET against the async adoption.
	// With the home detached, "main" is now unambiguously that single managed
	// worktree (the home carries branch=="").
	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	adopted := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "main"
	})
	wsID, _ := adopted["id"].(string)
	require.NotEmpty(t, wsID, "import must provision and broadcast the main managed worktree")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		repoPath:    repoPath,
	}
}

// importProjectHomeHoldsDefault imports a repo whose home STAYS on its protected
// default branch ("main"): unlike importProject it does NOT detach the home, so
// the home is adopted as the single isDefault workspace on "main" AND that same
// branch is surfaced as a locked, worktree-less PLACEHOLDER held by the repo
// folder (spec §3.4). It waits until BOTH the home and the placeholder have
// materialised so the placeholder-asserting tests never race the async import.
// The returned workspaceID is the home (isDefault) row; these tests read the
// repo's workspace list directly rather than operating on a managed worktree.
func importProjectHomeHoldsDefault(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t) // home stays on "main" (no detach)

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	var homeID string
	sawPlaceholder := false
	// Each frame's arrival is the signal; loop until BOTH the home and the main
	// placeholder have been seen. No wall-clock bound: readUntil blocks, so a
	// workspace that never materialises parks the read and `go test -timeout`
	// names this fixture in the goroutine dump.
	for homeID == "" || !sawPlaceholder {
		readUntil(t, workspacesWS, func(m map[string]any) bool {
			id, _ := m["id"].(string)
			if id == "" {
				return false
			}
			if m["isDefault"] == true {
				homeID = id
				return true
			}
			held, _ := m["heldByPath"].(string)
			if m["branch"] == "main" && held != "" {
				sawPlaceholder = true
				return true
			}
			return false
		})
	}

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: homeID,
		repoPath:    repoPath,
	}
}

// createProjectAndRepo runs the two-step public import flow (POST /v0/projects
// then POST .../repos) for an on-disk repo at repoPath and returns the project
// and repo ids learned from the hierarchical WS streams (dial-before-POST).
func createProjectAndRepo(
	t *testing.T,
	h *harness,
	repoPath string,
) (projectID, repoID string) {
	t.Helper()
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == repoPath
	})
	projectID, _ = project["id"].(string)
	require.NotEmpty(t, projectID, "create must broadcast a ProjectDTO with an id")

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	addResp := h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = addResp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ = repo["id"].(string)
	require.NotEmpty(t, repoID, "add-repo must broadcast a RepoDTO with an id")
	return projectID, repoID
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

	// The frame above came off the HUB projection; the store/list read model is an
	// INDEPENDENT projection that settles out of band (see
	// repositories.Container.WaitQuiescent). Observing the frame therefore says
	// nothing about whether the row is listable yet, so without this barrier the
	// fixture hands back an id the list may not know — and the loser shows up as
	// "imported workspace missing from list", or, when it is the DAEMON that reads
	// the list (DeleteCascade indexes it by id and returns ErrNotFound), as a
	// broadcast that never carries status "deleted" at all. Quiesce is the
	// deterministic read-your-writes barrier: every projection folded, no polling
	// and no timeout. macOS wins this race; Linux loses it roughly 7 times in 10.
	h.Quiesce()

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
	ForkPointSha    string `json:"forkPointSha"`
	FolderID        string `json:"folderId"`
	Order           int    `json:"order"`
	IsDefault       bool   `json:"isDefault"`
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
