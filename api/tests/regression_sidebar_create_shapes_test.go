//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests post EXACTLY the bodies the sidebar builds (space-content-actions.ts
// handleCreate/confirmPendingCreateName -> agent-api.ts createChat /
// createChatWithOwnWorktree) for each row kind, against a repo imported through
// the public flow, so a "failed" badge in the UI can be split into "the daemon
// refuses this shape" vs "the frontend never sends / mishandles it".

func workspaceByID(t *testing.T, h *harness, imported importedRepo, wsID string) *workspaceDTO {
	t.Helper()
	var rows []workspaceDTO
	h.get(repoBase(imported)+"/workspaces", &rows)
	for i := range rows {
		if rows[i].ID == wsID {
			return &rows[i]
		}
	}
	return nil
}

func forkFromSidebar(t *testing.T, h *harness, imported importedRepo, parentID, branch string) (string, *workspaceDTO) {
	t.Helper()
	body := map[string]any{"provider": "promotestub", "parentId": parentID, "ownWorktree": true}
	if branch != "" {
		body["branch"] = branch
	}
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats", body, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()
	h.Quiesce()
	detail := getAgentChat(t, h, repoBase(imported), created.ID)
	require.NotEmpty(t, detail.WorkspaceID, "fork must own a workspace")
	assert.Equal(t, parentID, detail.ParentID, "fork must be placed under the clicked row")
	ws := workspaceByID(t, h, imported, detail.WorkspaceID)
	require.NotNil(t, ws, "the new workspace must be listed by GET .../workspaces")
	if branch != "" {
		assert.Equal(t, branch, ws.Branch)
	}
	assert.Equal(t, created.ID, ws.OwningChatID, "the workspace list must name the fork's chat as owner")
	return created.ID, ws
}

// K2: fork off the LOCKED default branch row (the row the sidebar draws with the
// id of main's owning chat), with the slash name the user typed and a plain one.
func TestRegression_SidebarForkOffLockedDefaultBranch(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
	_, ws2 := forkFromSidebar(t, h, imported, imported.chatID, "feat-x")
	assert.Equal(t, imported.workspaceID, ws2.ParentID)
}

// K2: fork off an ORDINARY (unlocked) fork's row. rows-from-repo ids that row
// by its WORKSPACE id; handleCreate resolves owningChatId off the workspace
// record, so the daemon sees the owning chat id.
func TestRegression_SidebarForkOffOrdinaryFork(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
}

// K2: fork off the repo HEADER row when the home (isDefault) workspace still
// holds the default branch. The header is drawn from the default workspace;
// its owning chat is what the sidebar sends as parentId.
func TestRegression_SidebarForkOffRepoHeaderHoldingDefault(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
}

// K2 fallback shape: when the default workspace has NO owning chat the sidebar
// falls back to the WORKSPACE id itself (resolveHomeOwnerId -> homeId).
func TestRegression_SidebarForkWithWorkspaceIdAsParent(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	body := map[string]any{"provider": "promotestub", "parentId": imported.workspaceID, "ownWorktree": true, "branch": "test/test"}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.url+repoBase(imported)+"/chats", bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	t.Logf("status=%d body=%s", resp.StatusCode, raw)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// K1 fallback shape: a chatless workspace's row carries the WORKSPACE id, so a
// thread off it posts parentId=<workspaceId>.
func TestRegression_SidebarThreadWithWorkspaceIdAsParent(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	body := map[string]any{"provider": "promotestub", "parentId": imported.workspaceID, "workspaceId": imported.workspaceID}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.url+repoBase(imported)+"/chats", bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	t.Logf("status=%d body=%s", resp.StatusCode, raw)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	h.QuiesceReactors()
	h.Quiesce()
	detail := getAgentChat(t, h, repoBase(imported), env.Data.ID)
	assert.Equal(t, imported.workspaceID, detail.ParentID)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID)
	assert.NotEmpty(t, detail.LiveRunnerID)
	// and the row must be visible in the repo's chat list
	found := false
	for _, c := range listChats(t, h, imported.projectID, imported.repoID) {
		if c.ID == env.Data.ID {
			found = true
		}
	}
	assert.True(t, found, "the thread must be listed by GET .../chats")
}

// K1: thread off the locked default branch row and off a chat row — the body
// createChat builds: {provider, parentId, workspaceId}.
func TestRegression_SidebarThreadOffBranchAndChatRows(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	threadID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, imported.chatID)
	h.Quiesce()
	detail := getAgentChat(t, h, base, threadID)
	assert.Equal(t, imported.chatID, detail.ParentID)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID)

	nested := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, threadID)
	h.Quiesce()
	detail = getAgentChat(t, h, base, nested)
	assert.Equal(t, threadID, detail.ParentID)

	// thread off the fresh fork's own row
	forkID, ws := forkFromSidebar(t, h, imported, imported.chatID, "feat-x")
	onFork := createChatWithProvider(t, h, base, "promotestub", ws.ID, forkID)
	h.Quiesce()
	detail = getAgentChat(t, h, base, onFork)
	assert.Equal(t, forkID, detail.ParentID)
	assert.Equal(t, ws.ID, detail.WorkspaceID)
}

// K1: thread off the repo header (isDefault home workspace + its owning chat).
func TestRegression_SidebarThreadOffRepoHeaderHoldingDefault(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)
	base := repoBase(imported)

	threadID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, imported.chatID)
	h.Quiesce()
	detail := getAgentChat(t, h, base, threadID)
	assert.Equal(t, imported.chatID, detail.ParentID)
}

// K1: thread inside a folder (folder under the locked branch).
func TestRegression_SidebarThreadInsideFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	folder := createChatFolder(t, h, base, "holder", imported.chatID)
	threadID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, folder.ID)
	h.Quiesce()
	detail := getAgentChat(t, h, base, threadID)
	assert.Equal(t, folder.ID, detail.ParentID)

	// K2: fork inside that folder with the slash name
	forkFromSidebar(t, h, imported, folder.ID, "test/test")
}
