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

// BUG (workspace-with-no-owning-chat): a workspace could exist that no chat
// owned, which made it unreachable from anywhere in the product.
//
// The live example this was found from: the workspace behind
// `feature/pricing-rounding` had `owningChatId: ""`. It was a real, checked-out
// git worktree on disk, and nothing in the sidebar, the API or the daemon could
// open it — because nothing addresses a workspace except through the chat that
// owns it.
//
// Root cause: the import paths created the workspace row FIRST and minted no
// chat at all. The only thing that ever repaired them was BackfillOwningChats,
// a best-effort reconciler that runs at BOOT — so a workspace created during a
// daemon's life stayed orphaned for as long as that daemon ran.
//
// That boot-time detail is what these tests turn on, and it is why none of them
// restarts the daemon: a test that rebooted would be testing the reconciler,
// which already worked. Every assertion below is made against the SAME daemon
// that did the importing, so the only thing that can satisfy it is the write
// path itself refusing to produce an orphan.

// liveWorkspacesOf returns every workspace the daemon currently holds that has
// not been tombstoned.
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

// assertEveryWorkspaceIsOwned is the invariant itself, asserted over the whole
// daemon rather than over the one workspace a test happened to create: the bug
// was never about a particular branch, it was about a creation path that could
// leave ANY row unowned.
//
// It also checks the join from the other side. A chat that names a workspace no
// longer proves much on its own — the chat could be pointing at a row that was
// rolled back — so the owning chat is required to come back from the daemon's
// full chat list too, which is what the sidebar actually reads.
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

// Trigger 1 — the ad-hoc branch picker (POST .../chats/import-batch), the exact
// path the orphaned `feature/pricing-rounding` workspace came from.
func TestRegression_AnImportedBranchIsOwnedByAChatWithoutARestart(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/pricing-rounding"
	f.pushBranchToOrigin(t, branch, "main", "rounding\n")

	conn := h.dial(f.repoBase() + "/chats/ws")
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{branch}}, http.StatusAccepted).Body.Close()
	// No worktree_state frame ever reaches a freshly imported chat: pushChatWorktree
	// drops it whenever the chat's SetWorkspace event has not yet reached the
	// AgentChat projection (owningChatIDFor resolves "" and the push is skipped),
	// which is exactly the moment a create races into. workspace_set is the
	// reliable signal instead — SpawnChatWithImportedWorktree only fires it once
	// CreateImportedWorkspace has already materialised the workspace, so its
	// WorkspaceID is the real one.
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "workspace_set"
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
		"an ordinary unlocked branch owns a CHAT row — the same kind the boot backfill "+
			"gives an open worktree; only a LOCKED branch or a home owns a branch row")

	assertEveryWorkspaceIsOwned(t, h)
}

// A branch the import cannot materialise still lands as a PLACEHOLDER row, and
// that row is the one most likely to be left behind — so it needs an owner just
// as much. Here the repo home itself is holding the default branch, which git
// will not let a second worktree check out.
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

// Trigger 2 — repo add. This runs on EVERY repository a user adds, and it
// creates more workspaces than any other path: the repo home adopted in place,
// a locked managed worktree per protected branch, and the project-level home
// row that has no repo at all. None of them minted a chat before this change.
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

// The owning chat is not merely present, it is PLACED: a row filed nowhere is
// invisible in the panel even though the join says it exists. The repo's own
// rows hang at the panel root, and each one holds a distinct slot there — two
// rows claiming the same index is what makes the next drop land in the wrong
// place.
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
