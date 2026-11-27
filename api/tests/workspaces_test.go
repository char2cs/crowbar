//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaces_ListAndDetail(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	workspaces := listWorkspaces(t, h, imported.projectID, imported.repoID)
	require.NotEmpty(t, workspaces)
	assert.Equal(t, imported.workspaceID, findWorkspace(t, workspaces, imported.workspaceID).ID)

	detail := worktreeOf(t, h, imported, imported.workspaceID)
	assert.Equal(t, imported.workspaceID, detail.ID)
	assert.Equal(t, imported.repoID, detail.RepoID)
	assert.Equal(t, imported.projectID, detail.ProjectID)
	assert.Equal(t, "main", detail.Branch)
}

func findWorkspace(
	t *testing.T,
	workspaces []workspaceDTO,
	id string,
) workspaceDTO {
	t.Helper()
	for _, ws := range workspaces {
		if ws.ID == id {
			return ws
		}
	}
	t.Fatalf("workspace %s not found", id)
	return workspaceDTO{}
}
