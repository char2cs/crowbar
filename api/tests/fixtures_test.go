//go:build integration

package tests

import (
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
// API, and resolves the adopted workspace. Project import discovers the repo and
// adopts its main worktree as a workspace, so no explicit workspace create is
// needed for the happy path.
func importProject(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)

	var created struct {
		ID string `json:"id"`
	}
	h.post("/v0/projects", map[string]string{"name": "demo", "path": repoPath}, 201, &created)
	require.NotEmpty(t, created.ID)

	repos := listRepos(t, h, created.ID)
	require.Len(t, repos, 1, "import must discover exactly one repo")

	wsID := firstWorkspaceForRepo(t, h, repos[0].ID)
	return importedRepo{
		projectID:   created.ID,
		repoID:      repos[0].ID,
		workspaceID: wsID,
		repoPath:    repoPath,
	}
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
	h.get("/v0/repos?projectId="+projectID, &repos)
	return repos
}

type workspaceDTO struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	ProjectID string `json:"projectId"`
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
}

func firstWorkspaceForRepo(
	t *testing.T,
	h *harness,
	repoID string,
) string {
	t.Helper()
	var workspaces []workspaceDTO
	h.get("/v0/workspaces?repoId="+repoID, &workspaces)
	require.NotEmpty(t, workspaces, "import must adopt a workspace for the repo")
	return workspaces[0].ID
}

func writeFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
