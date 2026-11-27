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
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

var errImportBoom = errors.New("import: boom")

func importSpec(branch string) tree.ImportSpec {
	return tree.ImportSpec{RepoID: "repo-1", ProjectID: "prj-1", Branch: branch}
}

func TestCreateChat_ImportPlacesTheChatBeforeAttachingTheWorktree(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	chatID, _, err := createChat(context.Background(), uc, "", "claude", "c1",
		tree.WorktreeSpec{Mode: tree.WorktreeImport, Import: importSpec("feature/x")}, "")

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

func TestImportBranchAsChat_PlacesUnderTheChatOwningTheLineageParent(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "ws-base", Kind: dtree.KindWorkspace})
	chats.NextID = "c-new"

	spec := importSpec("feature/x")
	spec.ParentWorkspaceID = "ws-base"
	chatID, wsID, err := uc.ImportBranchAsChat(context.Background(), "", spec)

	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.NotEmpty(t, wsID, "the workspace id must come straight back from the create")
	assert.Equal(t, "ws-base", chatRow(t, chats, "c-new").ParentID,
		"the new row hangs off its lineage parent's own Node row directly")
}

func TestImportBranchAsChat_UnownedLineageParentLandsAtTheRoot(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	spec := importSpec("main")
	spec.ParentWorkspaceID = "ws-nobody-owns"
	chatID, _, err := uc.ImportBranchAsChat(context.Background(), "", spec)

	require.NoError(t, err)
	assert.Equal(t, "", chatRow(t, chats, chatID).ParentID)
}

func TestImportBranchAsChat_StartsNoRunner(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.ImportBranchAsChat(context.Background(), "", importSpec("feature/x"))

	require.NoError(t, err)
	require.Len(t, chats.SpawnedImportedWorktree, 1)
	assert.Empty(t, chats.SpawnedImportedWorktree[0].ProviderID,
		"no provider is named, so no CLI may be started")
	assert.Empty(t, chats.Started, "and the plain runner path is not taken either")
}

func TestImportBranchAsChat_RecordsTheDefaultProvider(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chats.DefaultProvider = "codex"

	_, _, err := uc.ImportBranchAsChat(context.Background(), "", importSpec("feature/x"))

	require.NoError(t, err)
	assert.Equal(t, []string{"codex"}, chats.MintedProviders)
	assert.Empty(t, chats.SpawnedImportedWorktree[0].ProviderID, "recorded, never launched")
}

func TestImportBranchAsChat_AFailedAttachKeepsTheChatForThePlaceholder(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.SpawnImportedWorktreeErr = errImportBoom

	chatID, _, err := uc.ImportBranchAsChat(context.Background(), "c-client", importSpec("feature/x"))

	require.ErrorIs(t, err, errImportBoom)
	assert.Equal(t, "c-client", chatID, "the chat is minted under the id the caller drew it as")
	assert.Empty(t, chats.Purged)
}

func TestCreateChat_AFailedImportDiscardsTheChat(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.SpawnImportedWorktreeErr = errImportBoom

	_, err := uc.CreateChat(context.Background(), tree.CreateChatInput{
		ID: "c-client", ProviderID: "claude",
		Worktree: tree.WorktreeSpec{Mode: tree.WorktreeImport, Import: importSpec("feature/x")},
	})

	require.ErrorIs(t, err, errImportBoom)
	assert.Equal(t, []string{"c-client"}, chats.Purged)
}

func TestMintOwningChat_MintsAndFilesTheRowBeforeAnyWorkspaceExists(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "ws-base", Kind: dtree.KindWorkspace})
	chats.NextID = "c-new"

	chatID, err := uc.MintOwningChat(context.Background(), "", "ws-base")

	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, []string{""}, chats.Minted,
		"minted in the workspace-less scope: the row is a bubble until the caller's workspace exists")
	assert.Equal(t, "ws-base", chatRow(t, chats, "c-new").ParentID,
		"placed directly under the parent workspace's own Node row")
	assert.Empty(t, chatRow(t, chats, "c-new").WorkspaceID,
		"nothing is owned yet: the workspace does not exist until the caller builds it")
}

func TestMintOwningChat_RecordsTheDefaultProvider(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()

	chats.NextID, chats.DefaultProvider = "c-a", "claude"
	_, err := uc.MintOwningChat(ctx, "", "")
	require.NoError(t, err)
	chats.NextID, chats.DefaultProvider = "c-b", ""
	_, err = uc.MintOwningChat(ctx, "", "")
	require.NoError(t, err)

	assert.Equal(t, []string{"claude", ""}, chats.MintedProviders)
}

func TestMintOwningChat_GivesEachRowItsOwnSlotInTheLevel(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()

	chats.NextID = "c-a"
	first, err := uc.MintOwningChat(ctx, "", "")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, first, domain.Workspace{ID: "ws-a"}))

	chats.NextID = "c-b"
	second, err := uc.MintOwningChat(ctx, "", "")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, second, domain.Workspace{ID: "ws-b"}))

	assert.NotEqual(t, chatRow(t, chats, first).Order, chatRow(t, chats, second).Order,
		"two rows in one level must not hold the same index, or the next drop lands on top of one")
}

func TestAttachOwningWorkspace_PointsTheRowAtIt(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "", "")
	require.NoError(t, err)

	err = uc.AttachOwningWorkspace(context.Background(), chatID,
		domain.Workspace{ID: "ws-1", Status: domain.WorkspaceStatusLocked})

	require.NoError(t, err)
	assert.Equal(t, "ws-1", chatRow(t, chats, chatID).WorkspaceID)
}

func TestAttachOwningWorkspace_FailureIsReported(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "", "")
	require.NoError(t, err)
	chats.AttachWorkspaceErr = errImportBoom

	err = uc.AttachOwningWorkspace(context.Background(), chatID, domain.Workspace{ID: "ws-1"})

	require.ErrorIs(t, err, errImportBoom)
}

func TestDiscardOwningChat_TakesTheRowBackOut(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	chatID, err := uc.MintOwningChat(context.Background(), "", "")
	require.NoError(t, err)

	require.NoError(t, uc.DiscardOwningChat(context.Background(), chatID))
	assert.Equal(t, []string{chatID}, chats.Purged)
}
