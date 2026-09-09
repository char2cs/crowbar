package tree_test

// Coverage for PlaceWorkspace (2026-09-09 sidebar-placement-unification,
// workspace-placement fix): a LOCKED branch's own row, moved directly by its
// own workspace id — never through the chat that owns it (checkChatMove's
// same-workspace rule is deliberately NOT what this verb enforces; see
// checkWorkspaceMove's own doc). Every fixture here lives in move_test.go's
// same single-repo world (workspaceID/repoID) unless a case specifically
// needs a second repo.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// An ORDINARY, unlocked fork moves through this call exactly like a locked
// branch does — not gated on RendersAsBranch (see PlaceWorkspace's own doc
// for the regression this pins: the frontend fires this same call for every
// worktree-owning row's drag, locked or not, so refusing an unlocked one
// here would turn its old, invisible no-op into a loud 404).
func TestPlaceWorkspace_MovesAnOrdinaryUnlockedForkJustLikeALockedBranch(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("fork-1", repoID)
	gitStatus.SetBranch("fork-1", false)

	_, _, err := uc.PlaceWorkspace(context.Background(), "fork-1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	n := nodeRowFor(t, nodes, "fork-1")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
}

// The project's own HOME workspace is never a member of a repo's tree at
// all (RepoOf answers "" for it) — the one workspaceID this call still
// refuses, matching planTreeRowDrop's own defaultWorkspaceId guard
// (drop-actions.ts) that keeps the frontend from ever calling this for it.
func TestPlaceWorkspace_RefusesTheProjectHomeWorkspace(t *testing.T) {
	_, _, _, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("home-ws", "") // RepoOf answers "" -- the home-workspace convention.

	_, _, err := uc.PlaceWorkspace(context.Background(), "home-ws", tree.PlaceInput{Order: index(0)})

	require.ErrorIs(t, err, apperr.ErrNotFound)
}

// A workspaceID RepoOf cannot resolve at all (never Set on the fake, the
// same convention a genuinely nonexistent id hits on the real adapter) is
// refused too, not silently treated as the home workspace.
func TestPlaceWorkspace_RefusesAnUnknownWorkspaceID(t *testing.T) {
	_, _, _, _, uc := newWorkspacePlacementUsecase(t)

	_, _, err := uc.PlaceWorkspace(context.Background(), "no-such-workspace", tree.PlaceInput{Order: index(0)})

	require.ErrorIs(t, err, apperr.ErrNotFound)
}

// THE adversarial case this fix exists for (mirrors
// TestPlaceChat_Home_LoneChatWithNoNodeRowStillMintsWhenTargetCoincidesWithZero
// and project.go's own repo-side regression): a locked branch whose Node row
// has never been touched -- a pre-existing branch older than the Node
// migration itself, or one whose RendersAsBranch just went true for the
// first time -- must still mint on its first placement, even when the
// requested index (0) happens to coincide with the zero-value ParentID/Order
// a never-touched row already carries. A diff-only write (place()'s own
// mechanism, project.go) would see no change and silently do nothing; this
// path goes through globalSnapshotAround's freshIDs instead, which does not
// have that blind spot.
func TestPlaceWorkspace_MintsNodeOnFirstTouch(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	// Deliberately no nodes.Rows entry for "branch-1".

	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err, "a locked branch with no Node row must mint one, not silently do nothing")

	n := nodeRowFor(t, nodes, "branch-1")
	assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
	assert.Equal(t, "", n.ParentID)
	assert.Equal(t, 0, n.Order)
}

// A locked branch may be filed into one of its OWN repo's folders -- the
// same sibling space checkFolderContainer already shares between folders,
// chats and (since this fix) branches.
func TestPlaceWorkspace_MovesIntoAFolderWithinItsOwnRepo(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "docs", "")
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("docs")})
	require.NoError(t, err)

	assert.Equal(t, "docs", placed.ParentID)
	assert.Equal(t, "docs", nodeRowFor(t, nodes, "branch-1").ParentID)
}

// The identical golden rule checkFolderContainer already enforces for a
// folder: a container from a DIFFERENT repo is refused, never silently
// crossed into.
func TestPlaceWorkspace_RefusesACrossRepoContainer(t *testing.T) {
	chats, _, _, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "f-other-repo", Type: domain.ChatTypeFolder, RepoID: "repo-2",
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	_, _, err := uc.PlaceWorkspace(
		context.Background(), "branch-1", tree.PlaceInput{ParentID: name("f-other-repo")})

	require.ErrorIs(t, err, tree.ErrCrossRepo)
}

// Pins the exact design hazard checkWorkspaceMove's own doc calls out:
// nearestWorkspaceAnchor(branchID) resolves to branchID itself before it
// ever walks up (its own Node.Kind already reads NodeKindWorkspace), so
// reusing checkFolderMove's unconditional checkFolderContextMove call would
// compare the branch's own id against the destination's answer and refuse
// EVERY legal move with ErrCrossContext -- including the most ordinary one,
// landing back at the bare repo root. This proves that never happens.
func TestPlaceWorkspace_MovingToTheBareRepoRootNeverCrossesContext(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "docs", "")
	// A reachable prior position -- "docs" is a real, discoverable row, unlike
	// a made-up parent id mergeForest's BFS would never reach from "" or any
	// known chat/folder id (which would mint a SECOND, unreachable Node row
	// instead of updating this one, a fixture bug rather than a real
	// production state: every real Node.ParentID names a container that was
	// itself validated by an earlier placement's own checkFolderContainer).
	nodes.Rows = append(nodes.Rows, domain.Node{
		ID: "branch-1", Kind: domain.NodeKindWorkspace, ParentID: "docs", Order: 0,
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Equal(t, "", placed.ParentID)
	assert.Equal(t, "", nodeRowFor(t, nodes, "branch-1").ParentID)
}

// A SECOND move reads the branch's live position off its own Node row, the
// same live-not-frozen contract PlaceChat's home-scoped path already keeps
// (TestPlaceChat_Home_SecondMoveReadsLivePositionFromNodeNotChat) -- and
// dispatches through Nodes.SetPlacement, never Nodes.Create again, once the
// row already exists.
func TestPlaceWorkspace_SecondMoveWritesPlacementNotCreate(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "folder-a", "")
	seedFolder(t, uc, "folder-b", "")
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("folder-a")})
	require.NoError(t, err)

	_, _, err = uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("folder-b")})
	require.NoError(t, err)

	assert.Equal(t, "folder-b", nodeRowFor(t, nodes, "branch-1").ParentID)
	require.NotEmpty(t, nodes.Placed)
	assert.Equal(t, "branch-1", nodes.Placed[len(nodes.Placed)-1].ID)
}

// A locked branch's move takes its whole subtree with it, exactly like a
// folder's -- so a working descendant refuses the move.
func TestPlaceWorkspace_RefusesWorkingSubtree(t *testing.T) {
	chats, work, _, gitStatus, uc := newWorkspacePlacementUsecaseWithWork(t)
	seedFolder(t, uc, "docs", "")
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "child-1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "branch-1",
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	work.Set("child-1", true)

	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("docs")})

	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

// newWorkspacePlacementUsecase builds the tree usecase with the Nodes fake
// and the fake WorkspaceGitStatus both exposed, for the PlaceWorkspace tests
// above that need to seed a Node row directly and/or configure
// RepoOf/RendersAsBranch.
func newWorkspacePlacementUsecase(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*mocks.FolderStore,
	*mocks.NodePlacements,
	*mocks.AgentWorkspaceGitStatus,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, inflight.NewWork(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, folders, nodes, gitStatus, uc
}

// newWorkspacePlacementUsecaseWithWork is newWorkspacePlacementUsecase with
// the in-flight tracker exposed, for TestPlaceWorkspace_RefusesWorkingSubtree.
func newWorkspacePlacementUsecaseWithWork(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*inflight.Work,
	*mocks.NodePlacements,
	*mocks.AgentWorkspaceGitStatus,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	work := inflight.NewWork()
	uc := tree.New(chats, chats, work, gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, work, nodes, gitStatus, uc
}
