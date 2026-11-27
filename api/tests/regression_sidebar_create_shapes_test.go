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

func TestRegression_SidebarForkOffLockedDefaultBranch(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
	_, ws2 := forkFromSidebar(t, h, imported, imported.chatID, "feat-x")
	assert.Equal(t, imported.workspaceID, ws2.ParentID)
}

func TestRegression_SidebarForkOffOrdinaryFork(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
}

func TestRegression_SidebarForkOffRepoHeaderHoldingDefault(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)

	_, ws := forkFromSidebar(t, h, imported, imported.chatID, "test/test")
	assert.Equal(t, imported.workspaceID, ws.ParentID)
}

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
	startChat(t, h, repoBase(imported), env.Data.ID)
	found := false
	for _, c := range listChats(t, h, imported.projectID, imported.repoID) {
		if c.ID == env.Data.ID {
			found = true
		}
	}
	assert.True(t, found, "the thread must be listed by GET .../chats")
}

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

	forkID, ws := forkFromSidebar(t, h, imported, imported.chatID, "feat-x")
	onFork := createChatWithProvider(t, h, base, "promotestub", ws.ID, forkID)
	h.Quiesce()
	detail = getAgentChat(t, h, base, onFork)
	assert.Equal(t, forkID, detail.ParentID)
	assert.Equal(t, ws.ID, detail.WorkspaceID)
}

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

	forkFromSidebar(t, h, imported, folder.ID, "test/test")
}
