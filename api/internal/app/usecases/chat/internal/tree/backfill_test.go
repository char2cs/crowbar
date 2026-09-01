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

// ownedRows is every chat row carrying wsID.
func ownedRows(
	chats *mocks.AgentChatPlacements,
	wsID string,
) []domain.Chat {
	owned := make([]domain.Chat, 0, 1)
	for _, row := range chats.Rows {
		if row.WorkspaceID == wsID {
			owned = append(owned, row)
		}
	}
	return owned
}

// ownedRow is the single chat row wsID owns. "Exactly one" is the invariant the
// backfill exists to establish, so both zero and two are a failure.
func ownedRow(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	wsID string,
) domain.Chat {
	t.Helper()
	owned := ownedRows(chats, wsID)
	require.Len(t, owned, 1, "workspace %s must own exactly one row", wsID)
	return owned[0]
}

// branchRow is the single BRANCH-typed row carrying wsID, whatever ordinary
// rows sit beside it.
func branchRow(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	wsID string,
) domain.Chat {
	t.Helper()
	branches := make([]domain.Chat, 0, 1)
	for _, row := range ownedRows(chats, wsID) {
		if row.Type == domain.ChatTypeBranch {
			branches = append(branches, row)
		}
	}
	require.Len(t, branches, 1, "workspace %s must own exactly one branch row", wsID)
	return branches[0]
}

// seedLegacyChat appends the row a pre-backfill install is full of: an ordinary
// conversation started inside a workspace, which carries that workspace's id
// exactly as an owning row would. It has been SPOKEN IN, which is what makes it
// a conversation rather than a structural row — the distinction the backfill's
// adopt decision turns on.
func seedLegacyChat(
	chats *mocks.AgentChatPlacements,
	id string,
	wsID string,
	createdAtSec int64,
) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID:          id,
		Type:        domain.ChatTypeChat,
		WorkspaceID: wsID,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	})
	if chats.Spoken == nil {
		chats.Spoken = map[string]bool{}
	}
	chats.Spoken[id] = true
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

// The gate asks a DIFFERENT question of a branch-destined workspace, and this is
// the case that forces it: a locked branch carrying ordinary conversations from
// before the backfill existed. Those rows carry its workspace id exactly as an
// owning row would — a chat started inside a workspace always has — but none of
// them is drawn as a branch row, so "some chat mentions this workspace" would
// leave the branch permanently without the row the sidebar addresses it by.
func TestBackfillOwningChats_MintsTheBranchRowALegacyChatDoesNotStandInFor(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
	})
	seedLegacyChat(chats, "legacy", "ws-locked", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	minted := branchRow(t, chats, "ws-locked")
	assert.NotEqual(t, "legacy", minted.ID, "the legacy conversation is not the branch row")
	assert.Len(t, ownedRows(chats, "ws-locked"), 2, "and it is left standing beside the new one")
}

// The other half of the same gate: once a branch row exists it is never minted
// again, whatever ordinary rows sit beside it.
func TestBackfillOwningChats_LeavesALockedWorkspaceThatAlreadyHasItsBranchRowAlone(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
	})
	seedLegacyChat(chats, "legacy", "ws-locked", 1)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "branch", Type: domain.ChatTypeBranch, WorkspaceID: "ws-locked",
		CreatedAt: time.Unix(2, 0).UTC(),
	})

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, "branch", branchRow(t, chats, "ws-locked").ID)
	assert.Len(t, ownedRows(chats, "ws-locked"), 2, "nothing new is minted")
}

// A child hangs off the BRANCH row, never off a legacy conversation that
// happens to predate it. Resolving to the older row would quietly make the
// child's workspace a thread of somebody's old chat.
func TestBackfillOwningChats_ChildrenHangOffTheBranchRowNotALegacyChat(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	seedWorkspace(roster, "ws-child", "ws-locked", 2)
	seedLegacyChat(chats, "legacy", "ws-locked", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, branchRow(t, chats, "ws-locked").ID, ownedRow(t, chats, "ws-child").ParentID)
}

// The same answer on the boot AFTER the branch row and the legacy chat start
// coexisting — the state this gate deliberately creates — where the branch row
// is no longer the one this pass minted but the one it reads back.
func TestBackfillOwningChats_ChildrenHangOffAnAlreadyStandingBranchRow(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	seedWorkspace(roster, "ws-child", "ws-locked", 2)
	// The legacy chat is the OLDER row, so a plain creation-order tiebreak picks
	// it over the branch row.
	seedLegacyChat(chats, "legacy", "ws-locked", 1)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "branch", Type: domain.ChatTypeBranch, WorkspaceID: "ws-locked",
		CreatedAt: time.Unix(9, 0).UTC(),
	})

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, "branch", ownedRow(t, chats, "ws-child").ParentID)
}

// A REGULAR workspace keeps the original gate. Any chat at all is already what
// resolveOwningChat treats as its owner, so minting a second row would recreate
// the ambiguity the gate exists to prevent.
func TestBackfillOwningChats_LeavesARegularWorkspaceWithALegacyChatAlone(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	seedWorkspace(roster, "ws-plain", "", 1)
	seedLegacyChat(chats, "legacy", "ws-plain", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Equal(t, "legacy", ownedRow(t, chats, "ws-plain").ID)
}

// What a workspace IS can change after its owning row is minted: an ordinary
// worktree backfilled as a chat row is LOCKED a week later, by the user or by
// the provider reporting its branch protected, and is branch-destined from then
// on. The next boot must ADOPT the row it already has rather than mint a second
// one beside it — two rows claiming one workspace is the exact invariant this
// whole backfill exists to establish.
func TestBackfillOwningChats_RetypesTheOwningRowOfAWorkspaceThatBecameLocked(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedWorkspace(roster, "ws-turned", "", 1)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	owned := ownedRow(t, chats, "ws-turned")
	require.Equal(t, domain.ChatTypeChat, owned.Type, "precondition: it was an ordinary worktree")

	roster.Rows[0].Status = domain.WorkspaceStatusLocked
	require.NoError(t, uc.BackfillOwningChats(ctx))

	after := ownedRow(t, chats, "ws-turned")
	assert.Equal(t, owned.ID, after.ID, "the SAME row, adopted — not a replacement")
	assert.Equal(t, domain.ChatTypeBranch, after.Type)
	assert.Equal(t, []mocks.TypeWrite{{ChatID: owned.ID, Type: domain.ChatTypeBranch}}, chats.Retyped)
}

// The retype leaves everything that is not the type exactly as it stands. A row
// re-placed instead of retyped would lose the level it was filed into, and a
// row replaced outright would take its children's parent id with it.
func TestBackfillOwningChats_RetypeKeepsThePlacementTheRowAlreadyHad(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedFolder(chats, "folder-1", "")
	seedWorkspace(roster, "ws-turned", "", 1)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	before := ownedRow(t, chats, "ws-turned")
	chats.Placed = nil

	roster.Rows[0].Status = domain.WorkspaceStatusLocked
	require.NoError(t, uc.BackfillOwningChats(ctx))

	after := ownedRow(t, chats, "ws-turned")
	assert.Equal(t, before.ParentID, after.ParentID)
	assert.Equal(t, before.Order, after.Order)
	assert.Empty(t, chats.Placed, "a retype writes no placement at all")
}

// Once adopted, the row satisfies the branch gate like any other branch row, so
// a third boot does nothing. Without this the retype would simply repeat.
func TestBackfillOwningChats_RetypesOnlyOnce(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedWorkspace(roster, "ws-turned", "", 1)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	roster.Rows[0].Status = domain.WorkspaceStatusLocked
	require.NoError(t, uc.BackfillOwningChats(ctx))
	chats.Retyped = nil

	require.NoError(t, uc.BackfillOwningChats(ctx))

	assert.Empty(t, chats.Retyped, "the third pass has nothing left to adopt")
	assert.Len(t, ownedRows(chats, "ws-turned"), 1)
}

// The adopt path stops at anything somebody has SPOKEN IN. A branch row does
// not open as a conversation, so adopting a row that holds words would hide
// them — the branch row is minted beside it instead. This is the round-1
// behaviour, and it is what keeps the retype above from reaching a real chat.
func TestBackfillOwningChats_AdoptsNothingThatHasBeenSpokenIn(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-locked", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
	})
	seedLegacyChat(chats, "legacy", "ws-locked", 1)

	require.NoError(t, uc.BackfillOwningChats(context.Background()))

	assert.Empty(t, chats.Retyped, "no conversation is turned into a branch row")
	assert.NotEqual(t, "legacy", branchRow(t, chats, "ws-locked").ID)
	assert.Equal(t, domain.ChatTypeChat, chatRow(t, chats, "legacy").Type,
		"the conversation keeps its kind and stays openable")
}

// A row nothing was ever said in is adopted even when it is not the only row
// carrying the workspace: an owning row the backfill minted for a worktree that
// has since been chatted in is still the row that owns it, and the conversation
// beside it is untouched.
func TestBackfillOwningChats_AdoptsTheSilentRowBesideASpokenOne(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedWorkspace(roster, "ws-turned", "", 1)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	owning := ownedRow(t, chats, "ws-turned")
	// The conversation starts AFTER the owning row exists, which is the only
	// order this case can happen in — you cannot chat in a workspace before it
	// has a row to chat in.
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "conversation", Type: domain.ChatTypeChat, WorkspaceID: "ws-turned",
		CreatedAt: time.Now().Add(time.Hour),
	})
	chats.Spoken = map[string]bool{"conversation": true}

	roster.Rows[0].Status = domain.WorkspaceStatusLocked
	require.NoError(t, uc.BackfillOwningChats(ctx))

	assert.Equal(t, owning.ID, branchRow(t, chats, "ws-turned").ID)
	assert.Len(t, ownedRows(chats, "ws-turned"), 2, "nothing new is minted")
	assert.Equal(t, domain.ChatTypeChat, chatRow(t, chats, "conversation").Type)
}

// A workspace that becomes locked while the daemon is RUNNING takes the same
// adopt decision, on the spot, without waiting for the next boot.
func TestEnsureOwningChat_AdoptsTheRowOfAWorkspaceLockedAtRuntime(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	ctx := context.Background()
	seedWorkspace(roster, "ws-turned", "", 1)
	require.NoError(t, uc.BackfillOwningChats(ctx))
	owned := ownedRow(t, chats, "ws-turned")

	roster.Rows[0].Status = domain.WorkspaceStatusLocked
	require.NoError(t, uc.EnsureOwningChat(ctx, roster.Rows[0]))

	after := ownedRow(t, chats, "ws-turned")
	assert.Equal(t, owned.ID, after.ID)
	assert.Equal(t, domain.ChatTypeBranch, after.Type)
}

// It is narrowed to the workspace it was asked about. The DECISION is still
// taken across the whole daemon — a row's parent is its fork parent's own row,
// which cannot be resolved for one workspace in isolation — but locking one
// branch must not quietly reconcile every other workspace on its way through.
func TestEnsureOwningChat_TouchesNoOtherWorkspace(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	seedWorkspace(roster, "ws-asked", "", 1)
	seedWorkspace(roster, "ws-bystander", "", 2)

	require.NoError(t, uc.EnsureOwningChat(context.Background(), roster.Rows[0]))

	assert.Len(t, ownedRows(chats, "ws-asked"), 1)
	assert.Empty(t, ownedRows(chats, "ws-bystander"), "the bystander is left for the boot pass")
}

// The narrowing keeps what the target's OWN placement depends on. A repo
// imported since the last boot has a whole chain nothing has backfilled, so a
// lock on a branch inside it plans a row whose ParentID is a row the same plan
// is about to mint for its fork parent — and dropping that parent's step would
// file the child under an id nothing answers to.
//
// The damage would be permanent, which is what makes this worth a test of its
// own: SetPlacement validates no parent, so the phantom is written happily, and
// alreadyOwned then reports the workspace satisfied on every later boot, so
// nothing ever repairs it.
func TestEnsureOwningChat_MintsTheAncestorRowsItsOwnPlacementNeeds(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows,
		domain.Workspace{
			ID: "ws-home", RepoID: repoID, IsDefault: true, CreatedAt: time.Unix(1, 0).UTC(),
		},
		domain.Workspace{
			ID: "ws-child", RepoID: repoID, ParentID: "ws-home",
			Status: domain.WorkspaceStatusLocked, CreatedAt: time.Unix(2, 0).UTC(),
		},
	)

	require.NoError(t, uc.EnsureOwningChat(context.Background(), roster.Rows[1]))

	home := ownedRow(t, chats, "ws-home")
	child := ownedRow(t, chats, "ws-child")
	assert.Equal(t, home.ID, child.ParentID, "the child hangs off a row that actually exists")
	assert.NotEmpty(t, child.ParentID, "and it is not left at the root either")
}

// The chain is pulled in as far as it goes, not one level.
func TestEnsureOwningChat_MintsAWholeUnownedAncestorChain(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows,
		domain.Workspace{
			ID: "ws-home", RepoID: repoID, IsDefault: true, CreatedAt: time.Unix(1, 0).UTC(),
		},
		domain.Workspace{
			ID: "ws-mid", RepoID: repoID, ParentID: "ws-home", CreatedAt: time.Unix(2, 0).UTC(),
		},
		domain.Workspace{
			ID: "ws-leaf", RepoID: repoID, ParentID: "ws-mid",
			Status: domain.WorkspaceStatusLocked, CreatedAt: time.Unix(3, 0).UTC(),
		},
	)

	require.NoError(t, uc.EnsureOwningChat(context.Background(), roster.Rows[2]))

	home := ownedRow(t, chats, "ws-home")
	mid := ownedRow(t, chats, "ws-mid")
	leaf := ownedRow(t, chats, "ws-leaf")
	assert.Equal(t, "", home.ParentID)
	assert.Equal(t, home.ID, mid.ParentID)
	assert.Equal(t, mid.ID, leaf.ParentID)
}

// A workspace locked at runtime that has never been backfilled at all still
// ends up owning a row — the ensure path mints as well as adopts.
func TestEnsureOwningChat_MintsWhenTheWorkspaceOwnsNothingYet(t *testing.T) {
	chats, uc, roster := newUsecaseWithRoster(t)
	roster.Rows = append(roster.Rows, domain.Workspace{
		ID: "ws-fresh", RepoID: repoID, Status: domain.WorkspaceStatusLocked,
	})

	require.NoError(t, uc.EnsureOwningChat(context.Background(), roster.Rows[0]))

	assert.Equal(t, domain.ChatTypeBranch, ownedRow(t, chats, "ws-fresh").Type)
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
