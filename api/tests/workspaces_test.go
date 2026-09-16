//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaces_ListAndDetail proves the workspace REST surface over a real
// imported repo: the adopted workspace appears in the repo-scoped list and
// resolves by id with its on-disk worktree path.
//
// The dedicated GET .../workspaces/:wsId detail route is gone (spec §8 step
// 6); worktreeOf reads the same answer off the chat list, which is where a
// worktree-owning chat's git state lives now.
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

// The standalone repo-scoped workspace WebSocket (dual-served .../workspaces,
// snapshot-on-connect) is deleted outright (spec §8 step 6): its replacement,
// the repo-scoped chat feed, is a live event stream with a deliberately nil
// Snapshot (agentChatDef) — TestWorkspaces_DualServeWSUpgrade asserted exactly
// the capability that was removed, so it is deleted rather than ported.

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
