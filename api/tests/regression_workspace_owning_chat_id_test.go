//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_WorkspaceDTOCarriesItsRealOwningChatID pins Task 5 of the
// 2026-09-01 owning-chat-backfill plan: the workspace wire DTO carries the id
// of the chat row the workspace actually owns, so a client (or Task 6's
// frontend) can address that row directly instead of independently deriving
// it — and, for a locked/default/home workspace, without re-deriving Task 3's
// branch-preference tiebreak a second, possibly-inconsistent way.
//
// The repo-home ("main", locked/protected) is planted with a LEGACY
// conversation from before the owning-chat backfill existed, deliberately, so
// a resolution that just took the first chat row (as the placement handler's
// own resolveOwningChat in hierarchy.go still does) would report the WRONG
// id here: the wire field must resolve to the branch row the backfill mints,
// exactly as Task 3's own preferred() tiebreak requires.
func TestRegression_WorkspaceDTOCarriesItsRealOwningChatID(t *testing.T) {
	home := t.TempDir()
	first := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, first)

	ctx := context.Background()
	rows, err := first.app.Repositories.Workspace.List(ctx)
	require.NoError(t, err)
	var lockedID string
	for _, ws := range rows {
		if ws.ProjectID == imported.projectID && ws.RepoID == imported.repoID &&
			ws.Status == domain.WorkspaceStatusLocked {
			lockedID = ws.ID
		}
	}
	require.NotEmpty(t, lockedID, "precondition: the repo-home main worktree must be locked")

	plantLegacyChat(t, first, lockedID)
	first.shutdown()

	second := newHarnessAt(t, home)
	second.Quiesce()

	lockedRows, err := second.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, lockedID)
	require.NoError(t, err)
	require.Len(t, lockedRows, 2, "the legacy conversation is kept and the backfilled branch row joins it")
	var branchID string
	for _, row := range lockedRows {
		if row.Type == domain.ChatTypeBranch {
			branchID = row.ID
		}
	}
	require.NotEmpty(t, branchID, "the locked workspace must have been backfilled a branch row")
	require.NotEqual(t, legacyChatID, branchID)

	childChat := owningChat(t, second, imported.workspaceID)
	require.Equal(t, domain.ChatTypeChat, childChat.Type, "precondition: the unlocked fork owns an ordinary chat row")

	// listWorkspaces (fixtures_test.go) is the wrong read here: its OwningChatID
	// is deliberately the CHAT ROW'S OWN id, not the nested resolved copy — sound
	// for one chat per workspace, but the locked workspace here is carried by TWO
	// (the branch row and the legacy conversation planted above), and collapsing
	// them into a byID map keeps whichever the loop saw last. The chat list's own
	// worktree.owningChatId is the field this test is actually about, and it
	// agrees across every row naming that workspace, so which one is read back
	// does not matter.
	var lockedOwner, childOwner string
	for _, c := range listChats(t, second, imported.projectID, imported.repoID) {
		switch c.WorkspaceID {
		case lockedID:
			require.NotNil(t, c.Worktree, "the locked repo-home's row must carry a worktree")
			lockedOwner = c.Worktree.OwningChatID
		case imported.workspaceID:
			require.NotNil(t, c.Worktree, "the unlocked fork's row must carry a worktree")
			childOwner = c.Worktree.OwningChatID
		}
	}

	assert.Equal(t, branchID, lockedOwner,
		"the locked row's wire owningChatId must resolve to the branch row, not the legacy conversation rows[0] would pick")
	assert.Equal(t, childChat.ID, childOwner,
		"a regular fork's wire owningChatId must resolve to its own chat-typed row")
}
