//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

func chatlessLockedBranch(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
) string {
	t.Helper()
	ctx := context.Background()
	repo, err := h.app.GORM.Repositories.FindByKey(ctx, imported.repoID)
	require.NoError(t, err)
	require.NotNil(t, repo)
	parent, err := h.app.Repositories.Workspace.Get(ctx, imported.workspaceID)
	require.NoError(t, err)
	own := true
	ws, err := h.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       imported.repoID,
		ProjectID:    imported.projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       branch,
		ParentID:     imported.workspaceID,
		ParentBranch: parent.Branch,
		ForceLocked:  true,
		OwnWorktree:  &own,
	})
	require.NoError(t, err)
	h.Quiesce()
	return ws.ID
}

func workspaceRow(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) workspaceDTO {
	t.Helper()
	var rows []workspaceDTO
	h.get(repoBase(imported)+"/workspaces", &rows)
	for _, w := range rows {
		if w.ID == wsID {
			return w
		}
	}
	require.Failf(t, "workspace missing", "GET /workspaces did not list %s", wsID)
	return workspaceDTO{}
}

func postChat(
	t *testing.T,
	h *harness,
	base string,
	body map[string]any,
) (int, string, string) {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.url+base+"/chats", bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	if env.Error == "" && !env.Success {
		env.Error = string(raw)
	}
	return resp.StatusCode, env.Data.ID, env.Error
}

func TestRegression_ForkFromChatlessLockedBranchForks(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	lockedID := chatlessLockedBranch(t, h, imported, "release/chatless")
	chatRows, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), lockedID)
	require.NoError(t, err)
	require.Empty(t, chatRows, "the row under test must be a chatless workspace")
	row := workspaceRow(t, h, imported, lockedID)
	require.Equal(t, "locked", row.Status)

	status, chatID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "parentId": lockedID, "ownWorktree": true, "branch": "test/test"})
	require.Equalf(t, http.StatusCreated, status,
		"forking a branch off a chatless locked branch must succeed; got %d: %s", status, errMsg)
	h.QuiesceReactors()

	detail := getAgentChat(t, h, base, chatID)
	assert.Equal(t, lockedID, detail.ParentID)
	assert.NotEmpty(t, detail.WorkspaceID)
	fork := workspaceRow(t, h, imported, detail.WorkspaceID)
	assert.Equal(t, lockedID, fork.ParentID, "the fork's git parent must be the locked branch it was cut from")
	assert.Equal(t, "test/test", fork.Branch)
}

func TestRegression_ThreadUnderChatlessLockedBranch(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	lockedID := chatlessLockedBranch(t, h, imported, "release/chatless-thread")

	status, chatID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "workspaceId": lockedID, "parentId": lockedID})
	require.Equalf(t, http.StatusCreated, status,
		"threading under a chatless locked branch must succeed; got %d: %s", status, errMsg)
	h.QuiesceReactors()

	detail := getAgentChat(t, h, base, chatID)
	assert.Equal(t, lockedID, detail.ParentID)
	assert.Equal(t, lockedID, detail.WorkspaceID)
	startChat(t, h, base, chatID)
}

func TestRegression_CreateUnderLegacyWorkspaceWithoutNodeRow(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)
	ctx := context.Background()

	lockedID := chatlessLockedBranch(t, h, imported, "release/legacy")
	require.NoError(t, h.app.Repositories.Placement.Forget(ctx, lockedID), "simulate a pre-Task-7 workspace: no Node row")
	h.Quiesce()

	status, _, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "workspaceId": lockedID, "parentId": lockedID})
	assert.Equalf(t, http.StatusCreated, status,
		"thread under a legacy (Node-less) locked branch; got %d: %s", status, errMsg)

	status, _, errMsg = postChat(t, h, base,
		map[string]any{"provider": "promotestub", "parentId": lockedID, "ownWorktree": true, "branch": "test/legacy"})
	assert.Equalf(t, http.StatusCreated, status,
		"fork off a legacy (Node-less) locked branch; got %d: %s", status, errMsg)
}
