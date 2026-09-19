//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

// chatlessFork cuts an UNLOCKED branch with no owning chat — a legacy
// ordinary fork, the one row kind the sidebar's trash accepts.
func chatlessFork(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
) string {
	t.Helper()
	ctx := context.Background()
	repo, err := h.app.GORM.Repositories.FindByKey(ctx, imported.repoID)
	require.NoError(t, err)
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
		OwnWorktree:  &own,
	})
	require.NoError(t, err)
	h.Quiesce()
	return ws.ID
}

// TestRegression_DeleteOwnerOfChatlessRowTakesAnchorFiledThreads: the sidebar
// files a thread under a chatless row by the WORKSPACE id (its rendered id),
// the first read then mints an owner, and the trash sends DELETE /chats/<owner>
// expecting the whole branch row (worktree + threads) to go.
func TestRegression_DeleteOwnerOfChatlessRowTakesAnchorFiledThreads(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	wsID := chatlessFork(t, h, imported, "feature/chatless-fork")

	status, threadID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "workspaceId": wsID, "parentId": wsID})
	require.Equalf(t, http.StatusCreated, status, "thread under chatless row: %d %s", status, errMsg)
	h.QuiesceReactors()

	row := workspaceRow(t, h, imported, wsID)
	require.NotEmpty(t, row.OwningChatID, "the read mints an owner")
	require.NotEqual(t, threadID, row.OwningChatID, "the thread is not hijacked as owner")
	ownerID := row.OwningChatID
	h.Quiesce()

	frames := dialAgentWS(t, h, base+"/chats/ws")
	resp := h.raw(http.MethodDelete, base+"/chats/"+ownerID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	waitForChatFrame(t, frames, ownerID, "deleted")
	h.Quiesce()

	var rows []workspaceDTO
	h.get(base+"/workspaces", &rows)
	for _, w := range rows {
		if w.ID != wsID {
			continue
		}
		assert.Equalf(t, "deleted", w.Status,
			"deleting the branch row's owner must reap the worktree; still live, re-minted owner %q (was %q)", w.OwningChatID, ownerID)
	}
	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	for _, c := range chats {
		assert.NotEqual(t, threadID, c.ID, "the thread filed under the branch row must go with it")
	}
}

// Same shape with a FORK filed under the anchor: the fork's worktree is
// cascaded away with its git parent while its chat survives, pointing at a
// deleted workspace.
func TestRegression_DeleteOwnerOfChatlessRowTakesAnchorFiledForks(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)
	ctx := context.Background()

	wsID := chatlessFork(t, h, imported, "feature/chatless-parent")

	status, forkID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "parentId": wsID, "ownWorktree": true, "branch": "feature/child"})
	require.Equalf(t, http.StatusCreated, status, "fork under chatless row: %d %s", status, errMsg)
	h.QuiesceReactors()
	fork := getAgentChat(t, h, base, forkID)
	require.Equal(t, wsID, fork.ParentID)
	require.NotEmpty(t, fork.WorkspaceID)

	row := workspaceRow(t, h, imported, wsID)
	require.NotEmpty(t, row.OwningChatID)
	ownerID := row.OwningChatID
	h.Quiesce()

	frames := dialAgentWS(t, h, base+"/chats/ws")
	resp := h.raw(http.MethodDelete, base+"/chats/"+ownerID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	waitForChatFrame(t, frames, ownerID, "deleted")
	h.Quiesce()

	// The delete reactor forgets a tombstone off the write path, so a reaped
	// workspace reads back as deleted or not at all.
	assertWorkspaceReaped(t, h, wsID, "the parent worktree is reaped")
	assertWorkspaceReaped(t, h, fork.WorkspaceID, "the fork's worktree goes with its git parent")
	_, err := h.app.Usecases.AgentChat.GetChat(ctx, forkID)
	assert.Error(t, err, "the fork's chat must go with its worktree, never survive pointing at a deleted workspace")
}

func assertWorkspaceReaped(t *testing.T, h *harness, wsID string, msg string) {
	t.Helper()
	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	if err != nil {
		return
	}
	assert.Equal(t, "deleted", string(ws.Status), msg)
}
