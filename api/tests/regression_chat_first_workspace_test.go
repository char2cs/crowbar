//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func liveWorkspacesOf(
	t *testing.T,
	h *harness,
) []domain.Workspace {
	t.Helper()
	h.Quiesce()
	all, err := h.app.Repositories.Workspace.List(context.Background())
	require.NoError(t, err)
	live := make([]domain.Workspace, 0, len(all))
	for _, ws := range all {
		if ws.Status != domain.WorkspaceStatusDeleted {
			live = append(live, ws)
		}
	}
	return live
}

func assertEveryWorkspaceIsOwned(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	ctx := context.Background()
	everyChat, err := h.app.Usecases.AgentChat.ListChats(ctx)
	require.NoError(t, err)
	listed := map[string]domain.Chat{}
	for _, c := range everyChat {
		listed[c.ID] = c
	}

	live := liveWorkspacesOf(t, h)
	require.NotEmpty(t, live, "the fixture must have created at least one workspace to prove anything")

	for _, ws := range live {
		owners, oErr := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, ws.ID)
		require.NoError(t, oErr)
		require.NotEmpty(t, owners,
			"workspace %s (branch %q, kind %q) owns no chat: it is reachable from nowhere, "+
				"which is the whole bug this path exists to make impossible",
			ws.ID, ws.Branch, ws.Kind)

		owner := owners[0]
		found, ok := listed[owner.ID]
		require.True(t, ok,
			"the chat owning workspace %s is not in the daemon's chat list, so the sidebar cannot draw it",
			ws.ID)
		assert.Equal(t, ws.ID, found.WorkspaceID,
			"the owning chat must point back at the workspace it owns")
	}
}

func TestRegression_AnImportedBranchIsOwnedByAChatWithoutARestart(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/pricing-rounding"
	f.pushBranchToOrigin(t, branch, "main", "rounding\n")

	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	created := feed.awaitNewChat(func(chat map[string]any) bool {
		return chat["workspaceId"] != ""
	})
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "import must broadcast a workspace for the branch")

	h.Quiesce()

	owners, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), wsID)
	require.NoError(t, err)
	require.Len(t, owners, 1,
		"the imported branch's workspace must own exactly one chat, minted in the same call — "+
			"no reboot has happened here, so a backfill cannot be what satisfies this")
	assert.Equal(t, wsID, owners[0].WorkspaceID)
	assert.Equal(t, domain.ChatTypeChat, owners[0].Type,
		"an ordinary unlocked branch owns a CHAT row — every workspace's owning chat is now, "+
			"regardless of lock status (2026-09-08 sidebar-placement-unification Task 9 retired "+
			"the retype-to-branch machinery; a locked/default/home workspace's own "+
			"Node{Kind:workspace} row is what the sidebar draws as a branch now)")

	assertEveryWorkspaceIsOwned(t, h)
}

func TestRegression_AnImportPlaceholderIsOwnedByAChatToo(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)

	h.Quiesce()

	live := liveWorkspacesOf(t, h)
	placeholders := 0
	for _, ws := range live {
		if ws.RepoID == imported.repoID && ws.WorktreePath == "" {
			placeholders++
		}
	}
	require.NotZero(t, placeholders,
		"precondition: the home holds the default branch, so it must land as a placeholder")

	assertEveryWorkspaceIsOwned(t, h)
}

func TestRegression_EveryWorkspaceARepoAddCreatesIsOwnedByAChat(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	h.Quiesce()

	live := liveWorkspacesOf(t, h)
	require.GreaterOrEqual(t, len(live), 2,
		"a repo add creates the project home and at least the repo's own rows")

	sawRepoRow := false
	sawProjectHome := false
	for _, ws := range live {
		if ws.RepoID == imported.repoID {
			sawRepoRow = true
		}
		if ws.Kind == domain.WorkspaceKindHome {
			sawProjectHome = true
		}
	}
	assert.True(t, sawRepoRow, "the added repo must have produced at least one workspace")
	assert.True(t, sawProjectHome,
		"the project-level home row is created by the same import and is covered by the same rule")

	assertEveryWorkspaceIsOwned(t, h)
}

func TestRegression_OwningChatsOfARepoAddAreFiledInDistinctSlots(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	h.Quiesce()
	ctx := context.Background()

	taken := map[int]string{}
	for _, ws := range liveWorkspacesOf(t, h) {
		if ws.RepoID != imported.repoID {
			continue
		}
		owners, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, ws.ID)
		require.NoError(t, err)
		require.NotEmpty(t, owners, "workspace %s owns no chat", ws.ID)
		owner := owners[0]
		if owner.ParentID != "" {
			continue
		}
		held, clash := taken[owner.Order]
		require.False(t, clash,
			"chats %s and %s both claim order %d at the panel root",
			held, owner.ID, owner.Order)
		taken[owner.Order] = owner.ID
	}
	require.NotEmpty(t, taken, "the repo add must have filed at least one row at the root")
}
