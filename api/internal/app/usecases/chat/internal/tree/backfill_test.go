package tree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newUsecaseWithRoster is newUsecase with the workspace census exposed, for the
// backfill tests below.
func newUsecaseWithRoster(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase, *mocks.AgentWorkspaceRoster) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	roster := mocks.NewAgentWorkspaceRoster()
	return chats, tree.New(chats, chats, inflight.NewWork(), mocks.NewAgentWorkspaceGitStatus(), roster), roster
}

// seedWorkspace appends a plain unlocked worktree to the census, born at the
// given second so the creation-order tiebreak is deterministic.
func seedWorkspace(
	roster *mocks.AgentWorkspaceRoster,
	id string,
	parentID string,
	bornAtSec int64,
) {
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID:        id,
		RepoID:    repoID,
		ParentID:  parentID,
		CreatedAt: time.Unix(bornAtSec, 0).UTC(),
	})
}

// ownedRow is the single chat row wsID owns. "Exactly one" is the invariant the
// backfill exists to establish, so both zero and two are a failure.
func ownedRow(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	wsID string,
) domain.Chat {
	t.Helper()
	owned := make([]domain.Chat, 0, 1)
	for _, row := range chats.Rows {
		if row.WorkspaceID == wsID {
			owned = append(owned, row)
		}
	}
	require.Len(t, owned, 1, "workspace %s must own exactly one row", wsID)
	return owned[0]
}

// A workspace's row hangs off its FORK PARENT's own row, which for a chain
// backfilled in one pass means the id minted moments earlier in the same call.
// Nothing can read that id back out of the store — the placement projection
// trails the write — so the pass has to carry it forward itself.
func TestBackfillOwningChats_HangsEachRowOffItsForkParentsOwnNewRow(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	// Seeded child-first: the census arrives in whatever order the read model
	// serves it, and the walk — not the input — is what puts roots first.
	seedWorkspace(roster, "ws-child", "ws-locked", 3)
	roster.Rows = append(roster.Rows,
		domain.Workspace{
			ID: "ws-locked", RepoID: repoID, ParentID: "ws-home",
			Status: domain.WorkspaceStatusLocked, CreatedAt: time.Unix(2, 0).UTC(),
		},
		domain.Workspace{
			ID: "ws-home", RepoID: repoID, IsDefault: true, CreatedAt: time.Unix(1, 0).UTC(),
		},
	)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	home := ownedRow(t, chats, "ws-home")
	locked := ownedRow(t, chats, "ws-locked")
	child := ownedRow(t, chats, "ws-child")
	assert.Equal(t, "", home.ParentID, "a fork-parentless workspace sits at the panel root")
	assert.Equal(t, home.ID, locked.ParentID)
	assert.Equal(t, locked.ID, child.ParentID)

	require.Len(t, chats.Placed, 3)
	assert.Equal(t, []string{home.ID, locked.ID, child.ID},
		[]string{chats.Placed[0].ChatID, chats.Placed[1].ChatID, chats.Placed[2].ChatID},
		"roots are written first, so no row is ever placed under an id that does not exist yet")
}

// What a workspace IS decides the kind of row it owns: a locked branch, a repo
// home and a project home are branch rows, an ordinary worktree is a chat.
func TestBackfillOwningChats_TypesEachRowFromWhatTheWorkspaceIs(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows,
		domain.Workspace{ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked},
		domain.Workspace{ID: "ws-default", RepoID: repoID, IsDefault: true},
		domain.Workspace{ID: "ws-home", Kind: domain.WorkspaceKindHome},
		domain.Workspace{ID: "ws-plain", RepoID: repoID},
	)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, domain.ChatTypeBranch, ownedRow(t, chats, "ws-locked").Type)
	assert.Equal(t, domain.ChatTypeBranch, ownedRow(t, chats, "ws-default").Type)
	assert.Equal(t, domain.ChatTypeBranch, ownedRow(t, chats, "ws-home").Type)
	assert.Equal(t, domain.ChatTypeChat, ownedRow(t, chats, "ws-plain").Type)
}

// The gate is per-workspace and it is what makes running this on every boot
// safe. A workspace that already owns a row keeps the row it has, and its
// children hang off THAT one rather than off a second copy.
func TestBackfillOwningChats_LeavesAWorkspaceThatAlreadyOwnsARowAlone(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "existing", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root",
	})
	seedWorkspace(roster, "ws-root", "", 1)
	seedWorkspace(roster, "ws-child", "ws-root", 2)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, "existing", ownedRow(t, chats, "ws-root").ID, "no second row is minted for it")
	assert.Equal(t, "existing", ownedRow(t, chats, "ws-child").ParentID,
		"a child resolves onto the row its parent already had")
}

// Running the backfill twice over the same daemon mints nothing the second
// time — the restart case, since it runs on every boot.
func TestBackfillOwningChats_MintsNothingOnASecondRun(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedWorkspace(roster, "ws-root", "", 1)
	seedWorkspace(roster, "ws-child", "ws-root", 2)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	first := len(chats.Rows)
	minted := ownedRow(t, chats, "ws-root").ID

	require.NoError(t, uc.BackfillOwningChats(ctx))

	assert.Len(t, chats.Rows, first, "the second run writes no rows at all")
	assert.Equal(t, minted, ownedRow(t, chats, "ws-root").ID, "and leaves the first run's row standing")
}

// Siblings sharing a parent are ordered by when their WORKSPACES were made, not
// by the order the census happened to list them in — the only tiebreak that is
// the same on every boot.
func TestBackfillOwningChats_OrdersSameParentSiblingsByCreationTime(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	seedWorkspace(roster, "ws-root", "", 1)
	// The ids run the OTHER way to the timestamps, so an accidental sort on the
	// id — or on the seeding order — answers the opposite of what is asserted.
	seedWorkspace(roster, "ws-a", "ws-root", 30)
	seedWorkspace(roster, "ws-b", "ws-root", 20)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, 0, ownedRow(t, chats, "ws-b").Order, "the older workspace takes the first slot")
	assert.Equal(t, 1, ownedRow(t, chats, "ws-a").Order)
}

// The backfilled rows join a sibling space that already has rows in it, so they
// land after them rather than on top of their indices.
func TestBackfillOwningChats_AppendsAfterTheRowsAlreadyAtThatLevel(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	seedFolder(chats, "folder-1", "")
	seedFolder(chats, "folder-2", "")
	seedWorkspace(roster, "ws-root", "", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, 2, ownedRow(t, chats, "ws-root").Order,
		"the two folders already at the root hold slots 0 and 1")
}

// A tombstoned workspace is on its way out — the boot sweep purges it moments
// before this runs — so minting a row onto it would leave a chat naming a
// workspace that is about to be gone.
func TestBackfillOwningChats_LeavesATombstonedWorkspaceAlone(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows,
		domain.Workspace{ID: "ws-live", RepoID: repoID},
		domain.Workspace{ID: "ws-gone", RepoID: repoID, Status: domain.WorkspaceStatusDeleted},
	)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	require.Len(t, chats.Rows, 1)
	assert.Equal(t, "ws-live", chats.Rows[0].WorkspaceID)
}

// A workspace whose recorded fork parent is gone is not stranded: it is rooted,
// so it still ends up owning a row the sidebar can address.
func TestBackfillOwningChats_RootsAWorkspaceWhoseForkParentIsGone(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	seedWorkspace(roster, "ws-orphan", "ws-vanished", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, "", ownedRow(t, chats, "ws-orphan").ParentID)
}

// A census that cannot be read is reported rather than silently treated as an
// empty daemon with nothing to backfill.
func TestBackfillOwningChats_ReportsAnUnreadableCensus(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Err = errNoLog

	err := uc.BackfillOwningChats(context.Background())

	assert.ErrorIs(t, err, errNoLog)
	assert.Empty(t, chats.Rows)
}
