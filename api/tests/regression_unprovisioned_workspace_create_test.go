//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// heldBranchPlaceholder imports a branch an EXTERNAL worktree already holds, so
// the import can only record a placeholder row: a workspace with no worktree on
// disk at all. It returns that workspace's id.
//
// The same flow TestRegression_RetryProvisionTakesRemoteContentNotDivergedLocal
// drives, reduced to the row it produces — the sidebar draws exactly this row
// with the amber "Branch needs provisioning" triangle.
func heldBranchPlaceholder(
	t *testing.T,
	h *harness,
	f importFixture,
	branch string,
) string {
	t.Helper()
	f.pushBranchToOrigin(t, branch, "main", "REMOTE\n")
	runGit(t, f.repoPath, "fetch", "origin", branch+":"+branch)
	runGit(t, f.repoPath, "worktree", "add", filepath.Join(t.TempDir(), "holder"), branch)

	conn := h.dial(f.repoBase() + "/chats/ws")
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{branch}}, http.StatusAccepted).Body.Close()
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "workspace_set"
	})
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "a branch held elsewhere must still produce a row")

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.Empty(t, ws.WorktreePath, "precondition: a held branch must arrive as a placeholder")
	return wsID
}

// TestRegression_CreateOnUnprovisionedWorkspace_IsRefused pins the create side
// of the placeholder row.
//
// Caught live: the sidebar offered Thread on the amber "Branch needs
// provisioning" row, the daemon answered 201, the runner spawned, and an
// ordinary-looking chat row and pane appeared — no toast, no failed badge. The
// only trace was the daemon log, where every read of that chat then 500'd
// ("agents: catalog worktree is invalid"), because the workspace the chat was
// attached to has no worktree for the CLI to run in. The refusal has to come
// BEFORE the mint, so nothing is created and the create-row error path has a
// real reason to show.
func TestRegression_CreateOnUnprovisionedWorkspace_IsRefused(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)

	wsID := heldBranchPlaceholder(t, h, f, "feature/held-placeholder")

	status, chatID, errMsg := postChat(t, h, f.repoBase(),
		map[string]any{"provider": "promotestub", "workspaceId": wsID})

	assert.Equalf(t, http.StatusConflict, status,
		"threading into a worktree-less branch must be refused, not 201'd; got %d: %s", status, errMsg)
	assert.Empty(t, chatID, "nothing may be minted for a workspace the chat cannot run in")
	assert.Contains(t, errMsg, "worktree", "the refusal must name the reason the client can show")
}

// TestRegression_CreateOnProvisionedWorkspace_StillSucceeds is the other half:
// the guard reads the worktree path, not the lock — an ordinary locked branch
// with a real worktree still takes threads.
func TestRegression_CreateOnProvisionedWorkspace_StillSucceeds(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	lockedID := chatlessLockedBranch(t, h, imported, "release/provisioned")
	require.NotEmpty(t, workspaceRow(t, h, imported, lockedID).LocalPath,
		"precondition: the branch is free on disk, so it must have a worktree")

	status, chatID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "workspaceId": lockedID, "parentId": lockedID})
	require.Equalf(t, http.StatusCreated, status,
		"a provisioned locked branch still takes a thread; got %d: %s", status, errMsg)
	assert.NotEmpty(t, chatID)
}
