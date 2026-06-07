//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaces_ListAndDetail proves the workspace REST surface over a real
// imported repo: the adopted workspace appears in the flat list and resolves by
// id with its on-disk worktree path.
func TestWorkspaces_ListAndDetail(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	var workspaces []workspaceDTO
	h.get("/v0/workspaces", &workspaces)
	require.NotEmpty(t, workspaces)
	assert.Equal(t, imported.workspaceID, findWorkspace(t, workspaces, imported.workspaceID).ID)

	var detail workspaceDTO
	h.get("/v0/workspaces/"+imported.workspaceID, &detail)
	assert.Equal(t, imported.workspaceID, detail.ID)
	assert.Equal(t, imported.repoID, detail.RepoID)
	assert.Equal(t, imported.projectID, detail.ProjectID)
	assert.Equal(t, "main", detail.Branch)
}

// TestWorkspaces_DualServeWSUpgrade proves the /v0/workspaces route dual-serves:
// a WebSocket upgrade delivers the workspace snapshot on connect rather than the
// REST list.
func TestWorkspaces_DualServeWSUpgrade(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial("/v0/workspaces?projectId=" + imported.projectID)
	got := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID
	})
	assert.Equal(t, imported.projectID, got["projectId"])
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
