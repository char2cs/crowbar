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

// chatlessLockedBranch cuts a LOCKED branch under imported's main worktree
// with NO owning chat — the exact row PR #181 restored GET .../workspaces for
// (a repo's default checkout, an untouched locked branch): every workspace a
// pre-chat-first build imported looks like this, and Task 9 deleted the boot
// backfill that used to mint a chat for it.
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

// postChat sends the create body exactly as the frontend does and reports the
// status plus the envelope, asserting nothing — the status IS the finding.
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

// TestRegression_ForkFromChatlessLockedBranchForks is K2 for a locked branch
// that owns no chat. The frontend's fork gesture (space-content-actions.ts
// handleCreate 'workspace') resolves placementParentId to the row's
// owningChatId and, when that is empty, to the workspace's OWN id — so the
// request the daemon receives is POST .../chats {ownWorktree, parentId:
// <workspaceId>, branch}. checkNewChatParent accepts that parent (the
// workspace's Node{Kind:workspace} row, workspaceAnchorView), the chat is
// minted and placed under it, and then SpawnChatWithOwnWorktree's
// ResolveForkParent walks a forest built from Chat rows + folders only
// (walk.go freshForest): the workspace id is neither a chat nor a tree node,
// CwdWorkspaceID breaks, ErrNoForkParent — the create is discarded and the
// pending row lands on "failed".
func TestRegression_ForkFromChatlessLockedBranchForks(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	lockedID := chatlessLockedBranch(t, h, imported, "release/chatless")
	// Chatless as the sidebar would have addressed it BEFORE any read minted
	// it an owner (TestRegression_ChatlessWorkspaceIsServedWithAnOwner): the
	// parent the client names is the workspace's own id either way.
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

// TestRegression_ThreadUnderChatlessLockedBranch is K1 for the same row: the
// thread gesture posts {workspaceId: <ws>, parentId: <ws>} (the branch row's
// rendered id IS the workspace id when it owns no chat).
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
	assert.NotEmpty(t, detail.LiveRunnerID)
}

// TestRegression_CreateUnderLegacyWorkspaceWithoutNodeRow is the pre-Task-7
// shape: a workspace minted by a build that never wrote Node{Kind:workspace}
// rows (no backfill exists). Both create gestures name it as parentId.
func TestRegression_CreateUnderLegacyWorkspaceWithoutNodeRow(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)
	ctx := context.Background()

	lockedID := chatlessLockedBranch(t, h, imported, "release/legacy")
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, lockedID), "simulate a pre-Task-7 workspace: no Node row")
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
