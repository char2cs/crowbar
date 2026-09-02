package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errImportBoom = errors.New("import: boom")

func importSpec(branch string) tree.ImportSpec {
	return tree.ImportSpec{RepoID: "repo-1", ProjectID: "prj-1", Branch: branch}
}

// THE ORDERING, for import. It is the same contract the fork path has and the
// same one the plain thread path has: the chat exists, and is placed, BEFORE
// the thing it owns. Import is the path that never did this — it created a
// workspace row with no chat anywhere — which is the whole bug (spec §0).
//
// ParentAtStart is recorded INSIDE the spawn, so this fails if the placement
// moves after it. The end state is identical either way, which is exactly why
// asserting the end state would prove nothing.
func TestCreateChat_ImportPlacesTheChatBeforeAttachingTheWorktree(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	chatID, _, err := uc.CreateChat(context.Background(), "", "claude", "c1",
		tree.WorktreeSpec{Mode: tree.WorktreeImport, Import: importSpec("feature/x")})

	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, []mocks.StartCall{{
		ChatID:        "c-new",
		ProviderID:    "claude",
		ParentAtStart: "c1",
	}}, chats.SpawnedImportedWorktree,
		"the worktree is attached only once the row is where it belongs")
	assert.Equal(t, []string{"feature/x"}, chats.ImportedBranches,
		"the branch the caller named must reach the attach")
}

// An import resolves WHERE it hangs from its git lineage, because that is the
// only thing a batch importer knows: it walked a PR-base graph and came out
// with a parent WORKSPACE, not a parent conversation. Translating that into the
// chat that owns it is this package's job (§7.6), and no caller's.
func TestImportBranchAsChat_PlacesUnderTheChatOwningTheLineageParent(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "owner-of-base", Type: domain.ChatTypeBranch, WorkspaceID: "ws-base",
	})
	chats.NextID = "c-new"

	spec := importSpec("feature/x")
	spec.ParentWorkspaceID = "ws-base"
	chatID, wsID, err := uc.ImportBranchAsChat(context.Background(), spec)

	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.NotEmpty(t, wsID, "the workspace id must come straight back from the create")
	assert.Equal(t, "owner-of-base", chatRow(t, chats, "c-new").ParentID,
		"the new row hangs off the chat that owns its lineage parent's workspace")
}

// A lineage parent nothing owns yet — and an import rooted at the repo, which
// has none at all — lands at the panel root rather than being refused.
func TestImportBranchAsChat_UnownedLineageParentLandsAtTheRoot(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	spec := importSpec("main")
	spec.ParentWorkspaceID = "ws-nobody-owns"
	chatID, _, err := uc.ImportBranchAsChat(context.Background(), spec)

	require.NoError(t, err)
	assert.Equal(t, "", chatRow(t, chats, chatID).ParentID)
}

// A batch import must not launch a vendor CLI per branch: these are rows, not
// conversations anybody opened.
func TestImportBranchAsChat_StartsNoRunner(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.ImportBranchAsChat(context.Background(), importSpec("feature/x"))

	require.NoError(t, err)
	require.Len(t, chats.SpawnedImportedWorktree, 1)
	assert.Empty(t, chats.SpawnedImportedWorktree[0].ProviderID,
		"no provider is named, so no CLI may be started")
	assert.Empty(t, chats.Started, "and the plain runner path is not taken either")
}

// A LOCKED import — every protected branch — owns a BRANCH row, and the
// judgement is taken from the workspace that came back rather than restated
// here. An ordinary branch stays a chat row (the next test).
func TestImportBranchAsChat_RetypesALockedImportAsABranchRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chats.ImportedWorkspace = domain.Workspace{
		ID: "ws-locked", Status: domain.WorkspaceStatusLocked,
	}

	chatID, _, err := uc.ImportBranchAsChat(context.Background(), importSpec("main"))

	require.NoError(t, err)
	assert.Equal(t, []mocks.TypeWrite{{ChatID: chatID, Type: domain.ChatTypeBranch}}, chats.Retyped)
}

func TestImportBranchAsChat_LeavesAnOrdinaryBranchAsAChatRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chats.ImportedWorkspace = domain.Workspace{ID: "ws-open"}

	_, _, err := uc.ImportBranchAsChat(context.Background(), importSpec("feature/x"))

	require.NoError(t, err)
	assert.Empty(t, chats.Retyped, "an unlocked worktree owns an ordinary chat row")
}

// A failed attach must take the chat back out. Leaving it behind would strand a
// placed, workspace-less row for a create the caller was told had failed.
func TestImportBranchAsChat_AttachFailureDiscardsTheChat(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chats.SpawnImportedWorktreeErr = errImportBoom

	_, _, err := uc.ImportBranchAsChat(context.Background(), importSpec("feature/x"))

	require.ErrorIs(t, err, errImportBoom)
	assert.Equal(t, []string{"c-new"}, chats.Purged,
		"a create the caller was told failed must not leave a chat behind")
}

// The three-verb primitive the paths that build their own workspace use. The
// mint has to come FIRST and has to be PLACED, or the row exists in no level.
func TestMintOwningChat_MintsAndFilesTheRowBeforeAnyWorkspaceExists(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "owner-of-base", Type: domain.ChatTypeBranch, WorkspaceID: "ws-base",
	})
	chats.NextID = "c-new"

	chatID, err := uc.MintOwningChat(context.Background(), "ws-base")

	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, []string{""}, chats.Minted,
		"minted in the workspace-less scope: the row is a bubble until the caller's workspace exists")
	assert.Equal(t, "owner-of-base", chatRow(t, chats, "c-new").ParentID)
	assert.Empty(t, chatRow(t, chats, "c-new").WorkspaceID,
		"nothing is owned yet: the workspace does not exist until the caller builds it")
}

// Two owning rows minted into the SAME level must not both claim slot 0.
//
// This is the failure the ""-scope densify produced: an owning row leaves the
// workspace-less scope as soon as it is attached, so the second mint planning
// against that scope could not see the first, and a repo import filed every one
// of its rows at index 0. See placeOwningRow.
func TestMintOwningChat_GivesEachRowItsOwnSlotInTheLevel(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()

	chats.NextID = "c-a"
	first, err := uc.MintOwningChat(ctx, "")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, first, domain.Workspace{ID: "ws-a"}))

	chats.NextID = "c-b"
	second, err := uc.MintOwningChat(ctx, "")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, second, domain.Workspace{ID: "ws-b"}))

	assert.NotEqual(t, chatRow(t, chats, first).Order, chatRow(t, chats, second).Order,
		"two rows in one level must not hold the same index, or the next drop lands on top of one")
}

// AttachOwningWorkspace points the row at what the caller built, and retypes it
// when the workspace turns out to be one the sidebar draws as a branch.
func TestAttachOwningWorkspace_PointsTheRowAtItAndRetypesALockedOne(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "")
	require.NoError(t, err)

	err = uc.AttachOwningWorkspace(context.Background(), chatID,
		domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusLocked})

	require.NoError(t, err)
	assert.Equal(t, "ws-1", chatRow(t, chats, chatID).WorkspaceID)
	assert.Equal(t, []mocks.TypeWrite{{ChatID: chatID, Type: domain.ChatTypeBranch}}, chats.Retyped)
}

// A failed attach is REPORTED, never swallowed: the caller is holding a
// workspace row nothing owns, and only an error tells it to roll that row back.
func TestAttachOwningWorkspace_FailureIsReported(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "")
	require.NoError(t, err)
	chats.AttachWorkspaceErr = errImportBoom

	err = uc.AttachOwningWorkspace(context.Background(), chatID, domain.Workspace{ID: "ws-1"})

	require.ErrorIs(t, err, errImportBoom)
}

// DiscardOwningChat is the compensating half the callers use when their own
// workspace create fails after the chat was already minted.
func TestDiscardOwningChat_TakesTheRowBackOut(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "")
	require.NoError(t, err)

	require.NoError(t, uc.DiscardOwningChat(context.Background(), chatID))
	assert.Equal(t, []string{chatID}, chats.Purged)
}
