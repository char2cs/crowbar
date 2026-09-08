package tree_test

// Coverage for project-home (RepoID == "") folder CRUD and chat placement
// migrating onto domain.Folder + domain.Node (2026-09-08
// sidebar-placement-unification Task 5). Repo-scoped folder CRUD keeps its
// existing coverage in tree_test.go/move_test.go/chats_test.go, untouched.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// homeWorkspaceID is never registered against a repo via SetRepo, so
// AgentWorkspaceGitStatus.RepoOf answers "" for it -- the same convention the
// real home workspace's RepoOf answers (see project.go's own doc).
const homeWorkspaceID = "home-ws-1"

// newHomeUsecase builds the tree usecase with fresh Folder/Node fakes exposed,
// for the home-scoped (RepoID == "") tests below.
func newHomeUsecase(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*mocks.FolderStore,
	*mocks.NodePlacements,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, inflight.NewWork(), gitStatus, mocks.NewAgentWorkspaceRoster(),
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, folders, nodes, uc
}

// newHomeUsecaseWithGitStatus is newHomeUsecase with the fake
// WorkspaceGitStatus exposed, for the tests below that configure
// RepoIDsForHome (SDD review fix round 3).
func newHomeUsecaseWithGitStatus(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*mocks.NodePlacements,
	*mocks.AgentWorkspaceGitStatus,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, inflight.NewWork(), gitStatus, mocks.NewAgentWorkspaceRoster(),
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, nodes, gitStatus, uc
}

// nodeRowFor returns the Node row a fake NodePlacements holds for id, failing
// the test if there is none.
func nodeRowFor(
	t *testing.T,
	nodes *mocks.NodePlacements,
	id string,
) domain.Node {
	t.Helper()
	for _, n := range nodes.Rows {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("nodeRowFor: no Node row for %s", id)
	return domain.Node{}
}

// A home folder mints a domain.Folder row plus a domain.Node row -- never a
// ChatTypeFolder Chat row, unlike a repo-scoped folder (tree_test.go's
// TestCreate_AppendsAtTheEndOfTheSiblingSpace and friends).
func TestCreate_Home_MintsFolderPlusNode(t *testing.T) {
	chats, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()

	created, shifted, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	assert.Equal(t, "docs", created.Title)
	assert.Equal(t, domain.ChatTypeFolder, created.Type)
	assert.Equal(t, "", created.RepoID)
	assert.NotEmpty(t, created.ID)
	assert.Empty(t, shifted)

	require.Len(t, folders.Saved, 1)
	assert.Equal(t, created.ID, folders.Saved[0].ID)
	assert.Equal(t, "docs", folders.Saved[0].Name)
	assert.Equal(t, "", folders.Saved[0].RepoID)

	require.Len(t, nodes.Rows, 1)
	assert.Equal(t, created.ID, nodes.Rows[0].ID)
	assert.Equal(t, domain.NodeKindFolder, nodes.Rows[0].Kind)

	assert.Empty(t, chats.Rows, "a home folder is never a Chat row")
}

func TestCreate_Home_TrimsAndRefusesABlankName(t *testing.T) {
	_, folders, _, uc := newHomeUsecase(t)
	ctx := context.Background()

	created, _, err := uc.Create(ctx, tree.CreateInput{Name: "  docs  "})
	require.NoError(t, err)
	assert.Equal(t, "docs", created.Title)

	_, _, err = uc.Create(ctx, tree.CreateInput{Name: "   "})
	assert.ErrorIs(t, err, tree.ErrNameRequired)
	assert.Len(t, folders.Saved, 1, "the refused second create mints nothing beyond the first")
}

// The golden rule (checkFolderContainer) still applies to a home folder over
// the SAME unified snapshot a repo-scoped folder plans against: filing it
// under a REPO-scoped folder is refused, exactly as filing a repo-scoped
// folder under a home one already is (tree_test.go's
// TestCreate_RefusesAChatParentFromAnotherRepo).
func TestCreate_Home_RefusesUnderARepoScopedFolder(t *testing.T) {
	chats, _, _, uc := newHomeUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "repo-folder", Type: domain.ChatTypeFolder, RepoID: repoID})

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ParentID: "repo-folder", Name: "docs"})
	assert.ErrorIs(t, err, tree.ErrCrossRepo)
}

// A create that mints the Folder row but fails to place it takes the Folder
// row back out, mirroring the repo-scoped path's own discardFolder.
func TestCreate_Home_DiscardsOnPlacementFailure(t *testing.T) {
	_, folders, nodes, uc := newHomeUsecase(t)
	nodes.CreateErr = errors.New("disk full")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{Name: "docs"})
	require.Error(t, err)
	assert.Empty(t, folders.Saved, "the folder row is taken back out when placement fails")
}

// Renaming a home folder writes ONLY domain.Folder.Name -- its position
// (domain.Node) is untouched, and no Chat aggregate is ever consulted.
func TestRename_Home_UpdatesFolderNameOnly(t *testing.T) {
	chats, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	renamed, err := uc.Rename(ctx, created.ID, "  guides  ")
	require.NoError(t, err)
	assert.Equal(t, "guides", renamed.Title)

	require.Len(t, folders.Saved, 1)
	assert.Equal(t, "guides", folders.Saved[0].Name)
	assert.Equal(t, created.Order, nodeRowFor(t, nodes, created.ID).Order, "position untouched by a rename")
	assert.Empty(t, chats.Rows, "renaming a home folder never touches the chat aggregate")
}

func TestRename_Home_UnknownIDIsNotFound(t *testing.T) {
	_, _, _, uc := newHomeUsecase(t)
	_, err := uc.Rename(context.Background(), "nope", "new name")
	assert.Error(t, err)
}

// Moving a home folder reparents its domain.Node row and never touches the
// chat aggregate -- the direct opposite of a repo-scoped folder move
// (move_test.go's TestMove_TakesWholeSubtree), which writes through Chats.
func TestMove_Home_ReparentsViaNode(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	a, _, err := uc.Create(ctx, tree.CreateInput{ID: "a", Name: "a"})
	require.NoError(t, err)
	b, _, err := uc.Create(ctx, tree.CreateInput{ID: "b", Name: "b"})
	require.NoError(t, err)

	placed, _, err := uc.Move(ctx, a.ID, tree.MoveInput{ParentID: name(b.ID)})
	require.NoError(t, err)
	assert.Equal(t, b.ID, placed.ParentID)

	n := nodeRowFor(t, nodes, "a")
	assert.Equal(t, "b", n.ParentID)
	assert.Empty(t, chats.Placed, "a home move never writes the chat aggregate")
	assert.Empty(t, chats.Ordered, "a home move never writes the chat aggregate")
}

// A repo's own Node phantom row (mergeHomeForest's placeholder, purely so a
// repo densifies alongside its home siblings) is NOT a legal container for
// anything — hardened after an SDD review caught it reachable via a raw
// PATCH with no frontend involvement: checkFolderContainer's repoScopeOf
// used to answer nil ("nothing to conflict with") for it, the same posture
// correctly taken for a plain bubble sibling, silently letting a folder be
// filed under a repo and become permanently unreachable (repos are BFS
// leaves — nothing walks INTO one to find what was filed there).
func TestMove_Home_RefusesFilingUnderARepo(t *testing.T) {
	_, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, folder.ID, tree.MoveInput{ParentID: name("repo-1")})
	assert.ErrorIs(t, err, tree.ErrNotAContainer)
}

// The CHAT-placement half of the same hardening (checkParentKind, not
// checkFolderContainer) -- a home chat's parent may never resolve to a repo
// either.
func TestPlaceChat_Home_RefusesFilingUnderARepo(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0})
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})

	_, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name("repo-1")})
	assert.ErrorIs(t, err, tree.ErrNotAContainer)
}

// TestRegression_PlaceChat_Home_DoesNotCorruptAnotherProjectsRepoSharingTheBareRoot
// is project.go's TestRegression_UpdateRepo_BareRootReorderDoesNotCorruptAnotherProjectsHomeChats
// (Critical 2) in the REVERSE direction: a chat placement in project A's home
// must not renumber -- and WRITE, via Nodes.SetOrder/.SetPlacement -- project
// B's own repo Node row sharing the same literal "" root (SDD review fix
// round 3; mergeHomeForest's repo-phantom rows carry no project id of their
// own, the same underlying gap Critical 2 closed for CHAT-kind rows).
func TestRegression_PlaceChat_Home_DoesNotCorruptAnotherProjectsRepoSharingTheBareRoot(t *testing.T) {
	chats, nodes, gitStatus, uc := newHomeUsecaseWithGitStatus(t)
	ctx := context.Background()
	// Project A's home workspace (homeWorkspaceID) owns repo-A only --
	// repo-B belongs to a DIFFERENT project's home, sharing the bare root.
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "repo-B", Kind: domain.NodeKindRepo, Order: 1},
	}

	_, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-B").Order,
		"another project's repo sharing the bare root must be untouched")
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "repo-B", w.ID, "another project's repo must never be WRITTEN")
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "repo-B", w.ID, "another project's repo must never be WRITTEN")
	}
	// This project's OWN repo still correctly densifies against the chat
	// that just landed before it -- the fix scopes, it does not disable.
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-A").Order,
		"this project's own repo still shifts correctly")
	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
}

// The direct Node-native proof of Task 5's whole point: moving a home folder
// densifies against a REPO's own Node row sharing the exact same container,
// read from ONE Node.ListByParent call -- no cross-aggregate merge step (the
// interim fix Task 3 kept alive and this task deletes from project.go).
func TestMove_Home_DensifiesAgainstRepoNodeRowsToo(t *testing.T) {
	_, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0})
	_, _, err := uc.Create(ctx, tree.CreateInput{ID: "a", Name: "a"})
	require.NoError(t, err)
	require.Equal(t, 1, nodeRowFor(t, nodes, "a").Order, "a landed after the repo already at the root")

	_, shifted, err := uc.Move(ctx, "a", tree.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Empty(t, shifted, "the repo is not a FOLDER row, so it never rides the folder broadcast list")

	assert.Equal(t, 0, nodeRowFor(t, nodes, "a").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-1").Order,
		"the repo sibling shifted too, written back through the SAME Node surface")
}

// A folder delete PROMOTES what it held to its own parent -- a chat filed
// under it stays a chat, only its container closes up. Both the Folder row
// and the Node row are erased outright.
func TestDelete_Home_PromotesChildrenAndErasesFolderAndNode(t *testing.T) {
	_, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	parent, _, err := uc.Create(ctx, tree.CreateInput{ID: "parent", Name: "parent"})
	require.NoError(t, err)
	child, _, err := uc.Create(ctx, tree.CreateInput{ID: "child", ParentID: parent.ID, Name: "child"})
	require.NoError(t, err)

	_, err = uc.Delete(ctx, parent.ID)
	require.NoError(t, err)

	for _, f := range folders.Saved {
		assert.NotEqual(t, "parent", f.ID, "the deleted folder's identity row is gone")
	}
	assert.Equal(t, "", nodeRowFor(t, nodes, child.ID).ParentID, "the child is promoted to the root")
}

// The DIRECT counterpart of TestMove_Home_DensifiesAgainstRepoNodeRowsToo for
// a home-scoped CHAT: PlaceChat's position write goes through Node, densified
// against a repo AND a home folder sharing the same container in one read.
func TestPlaceChat_Home_DensifiesAgainstRepoAndFolderSiblings(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)
	require.Equal(t, 1, nodeRowFor(t, nodes, folder.ID).Order)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})

	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, domain.NodeKindChat, nodeRowFor(t, nodes, "c1").Kind)
	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-1").Order, "shifted by the chat landing before it")
	assert.Equal(t, 2, nodeRowFor(t, nodes, folder.ID).Order, "shifted too")

	assert.Empty(t, chats.Placed, "a home chat's position write never touches the chat aggregate")
	assert.Empty(t, chats.Ordered, "a home chat's position write never touches the chat aggregate")
}

// A home-scoped chat's FIRST placement mints its Node row (the same
// Create-vs-SetPlacement dispatch a fresh home folder gets) -- proving
// CreateChat's ordinary MintChat-then-PlaceChat sequence needs no special
// casing for home scope at all.
func TestPlaceChat_Home_FirstPlacementMintsANodeRow(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	placed, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folder.ID)})
	require.NoError(t, err)
	assert.Equal(t, folder.ID, placed.ParentID)

	n := nodeRowFor(t, nodes, "c1")
	assert.Equal(t, folder.ID, n.ParentID)
	assert.Equal(t, domain.NodeKindChat, n.Kind)
	assert.Empty(t, chats.Placed, "the write went to Node, never Chat.SetPlacement")
}

// Chat.ParentID/.Order are write-once-at-creation-then-ignored for a
// home-scoped chat (brief's own contract, not deleted until Task 11): a
// SECOND move reads its live position off Node (not the frozen Chat field)
// to compute the next densify correctly, and never restates the Chat field.
func TestPlaceChat_Home_SecondMoveReadsLivePositionFromNodeNotChat(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	folderA, _, err := uc.Create(ctx, tree.CreateInput{ID: "fa", Name: "a"})
	require.NoError(t, err)
	folderB, _, err := uc.Create(ctx, tree.CreateInput{ID: "fb", Name: "b"})
	require.NoError(t, err)

	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folderA.ID)})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folderB.ID)})
	require.NoError(t, err)

	assert.Equal(t, folderB.ID, nodeRowFor(t, nodes, "c1").ParentID, "Node tracks the live position")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID,
		"Chat's own field stays frozen at its creation-time value -- Task 11 deletes it, not this task")
	assert.Empty(t, chats.Placed)
	assert.Empty(t, chats.Ordered)
}

// ListInRepo("") reads home folders from Folder/Node, isolated from
// repo-scoped ChatTypeFolder rows -- the home half of tree_test.go's
// TestListInRepo_IsolatesByRepo, which used to seed this fixture directly
// onto chats.Rows before home folders left the Chat aggregate.
func TestListInRepo_Home_IsolatesFromRepoScoped(t *testing.T) {
	chats, _, _, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "f-repo", Type: domain.ChatTypeFolder, RepoID: repoID, Title: "repo-scoped"})
	created, _, err := uc.Create(ctx, tree.CreateInput{ID: "f-home", Name: "home"})
	require.NoError(t, err)

	rows, err := uc.ListInRepo(ctx, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, created.ID, rows[0].ID)
	assert.Equal(t, "home", rows[0].Title)
}

// DeletePreview resolves a home-scoped folder id too. A real gap this task's
// own migration opened and closed within it: delete_preview.go's bare
// Chats.LoadChat(chatID) read (unchanged since before this task) would
// refuse ANY home folder id as not-found the moment it stopped being a Chat
// row -- fixed by trying Folders first, mirroring Rename/Move/Delete's own
// dispatch.
func TestDeletePreview_Home_ResolvesAHomeFolderRoot(t *testing.T) {
	chats, _, _, uc := newHomeUsecase(t)
	ctx := context.Background()
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, ParentID: folder.ID, WorkspaceID: homeWorkspaceID})

	chatCount, fileCount, err := uc.DeletePreview(ctx, folder.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, chatCount, "the folder itself is not a chat")
	assert.Equal(t, 0, fileCount, "the chat inside owns no workspace of its own here")
}
