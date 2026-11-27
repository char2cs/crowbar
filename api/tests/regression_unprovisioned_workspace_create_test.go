//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	created := feed.awaitNewChat(hasWorkspace)
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "a branch held elsewhere must still produce a row")

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.Empty(t, ws.WorktreePath, "precondition: a held branch must arrive as a placeholder")
	return wsID
}

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
