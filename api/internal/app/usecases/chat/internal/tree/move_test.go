package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newUsecaseWithWork is newUsecase with the in-flight tracker exposed, for the
// tests below that need to mark a row working.
func newUsecaseWithWork(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase, *inflight.Work) {
	t.Helper()
	chats, _, _, uc, work := newUsecaseWithStores(t)
	return chats, uc, work
}

// newUsecaseWithStores is newUsecaseWithWork with the Folders/Nodes fakes
// ALSO exposed — 2026-09-08 sidebar-placement-unification Task 8, for the
// tests that need to seed a Node row directly or inject a Folders/Nodes
// store failure now that a folder's identity/position live there instead of
// on chats.Rows.
func newUsecaseWithStores(
	t *testing.T,
) (*mocks.AgentChatPlacements, *mocks.FolderStore, *mocks.NodePlacements, tree.Usecase, *inflight.Work) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	// Cross-referenced so StartCall.ParentAtStart's ordering proof (see
	// AgentChatPlacements.parentOf's own doc) can see a repo-scoped chat's
	// live Node position too, not just the Chat row's frozen field.
	chats.Nodes = nodes
	work := inflight.NewWork()
	// Every fixture in this file is a single-repo world: workspaceID ("ws-1")
	// belongs to repoID ("repo-1"), so a folder created/moved under a plain
	// CHAT parent (which carries workspaceID, not a repo id of its own)
	// resolves to the SAME repo every Create/Move call here already assumes —
	// see domain.Chat.RepoID / checkFolderContainer's golden rule.
	workspaceGitStatus := mocks.NewAgentWorkspaceGitStatus()
	workspaceGitStatus.SetRepo(workspaceID, repoID)
	uc := tree.New(chats, chats, work, workspaceGitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats),
		folders, nodes)
	return chats, folders, nodes, uc, work
}

// seedFolderTree creates "root" and "other" as sibling folders and files
// child-1 and child-2 as chats under "root", the fixture both Move tests below
// share.
func seedFolderTree(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	uc tree.Usecase,
) {
	t.Helper()
	ctx := context.Background()
	_, _, err := uc.Create(ctx, tree.CreateInput{ID: "root", RepoID: repoID, Name: "root"})
	require.NoError(t, err)
	_, _, err = uc.Create(ctx, tree.CreateInput{ID: "other", RepoID: repoID, Name: "other"})
	require.NoError(t, err)
	seedThread(chats, "child-1", "root", 1)
	seedThread(chats, "child-2", "root", 2)
}

// A folder move takes its whole subtree — a chat filed under it is part of
// what moves — so a working descendant refuses the move exactly as a working
// chat refuses its own.
func TestMove_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)
	work.Set("child-1", true)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

// The subtree travels without a single one of its rows being rewritten: a
// child's ParentID already names the row that moved, so re-parenting "root"
// carries "child-1" and "child-2" with it for free.
func TestMove_TakesWholeSubtree(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)

	placed, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	require.NoError(t, err)

	assert.Equal(t, "other", placed.ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-1").ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-2").ParentID)
}

// The folder-scoping golden rule at the OTHER usecase (Move, not Create): a
// folder's own repo scope is fixed at creation and never changes by being
// dragged — moving it under a folder from a DIFFERENT repo is refused, the
// same way ErrCrossWorkspace already refuses a cross-workspace CHAT move.
// Caught live as a folder left "on top of" the wrong repo after a cross-repo
// drag.
func TestMove_RefusesAMoveAcrossRepos(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc) // "root" and "other", both repoID
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "other-repo-folder", Type: domain.ChatTypeFolder, RepoID: "repo-2"},
	)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other-repo-folder")})
	assert.ErrorIs(t, err, tree.ErrCrossRepo)
}

// The golden rule's finer grain (checkFolderContextMove): "context", in
// order, is Project -> Repo -> Locked branch -> Parent unlocked branch — a
// folder may move freely WITHIN whichever one it already sits under, but
// never jump to a different one, even inside the SAME repo. These three
// tests exercise that boundary directly: refused root -> a branch's own
// subtree, refused branch -> a DIFFERENT branch's own subtree (same repo,
// same ErrCrossRepo-passing check, still refused), and allowed within one
// branch's own subtree. Caught live: dragging a repo-root folder onto a
// branch (or vice versa) silently reverted with no explanation — this is
// the check that was missing, not merely the toast that now names it.
//
// 2026-09-08 sidebar-placement-unification Task 9 re-ports this suite
// against nearestWorkspaceAnchor's simplified Node.Kind==NodeKindWorkspace
// check: a "branch" fixture is now a Node{Kind:workspace} row, seeded under
// the SAME id as the fixture chat that stands beside it (checkFolderContainer
// still needs a Chat/Folder-shaped row at that id to resolve a container —
// wiring a workspace's own Node row into that resolution for real is Task
// 10's job) so the anchor test recognizes it directly, without the
// branch-preference tiebreak the deleted boot backfill used to need.
func TestMove_RefusesRootToBranchContext(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedFolderTree(t, chats, uc) // "root", a repo-root folder — its own context
	seedChat(chats, "branch-1", 1)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "branch-1", Kind: domain.NodeKindWorkspace})

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("branch-1")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_RefusesBranchToDifferentBranchContext(t *testing.T) {
	chats := mocks.NewAgentChatPlacements()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"},
		domain.Chat{ID: "branch-2", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"},
	)
	nodes := mocks.NewNodePlacements()
	nodes.Rows = append(nodes.Rows,
		domain.Node{ID: "branch-1", Kind: domain.NodeKindWorkspace},
		domain.Node{ID: "branch-2", Kind: domain.NodeKindWorkspace},
	)
	// Both branches resolve to the SAME repo — isolates the finer context
	// check from the coarser repo check TestMove_RefusesAMoveAcrossRepos
	// already covers.
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	gitStatus.SetRepo("ws-1", repoID)
	gitStatus.SetRepo("ws-2", repoID)
	uc2 := tree.New(chats, chats, inflight.NewWork(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats),
		mocks.NewFolderStore(), nodes)
	underBranch1, _, err := uc2.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "notes",
	})
	require.NoError(t, err)

	_, _, err = uc2.Move(context.Background(), underBranch1.ID, tree.MoveInput{ParentID: name("branch-2")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_AllowsAMoveWithinTheSameBranchContext(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "branch-1", 1)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "branch-1", Kind: domain.NodeKindWorkspace})
	underBranch1, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "notes",
	})
	require.NoError(t, err)
	nestedDeeper, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "deeper",
	})
	require.NoError(t, err)

	// Re-nesting one folder inside another, both already anchored to the
	// SAME branch, stays within that one context — allowed.
	placed, _, err := uc.Move(context.Background(), underBranch1.ID, tree.MoveInput{
		ParentID: name(nestedDeeper.ID),
	})
	require.NoError(t, err)
	assert.Equal(t, nestedDeeper.ID, placed.ParentID)
}

// A folder filed INTO a plain chat SIBLING that already sits at the exact
// same panel root never leaves its context, even though that chat carries a
// real WorkspaceID of its own (every chat does — domain.Chat's own doc) and
// the folder does not. Neither "owner" nor "sibling" has a Node row of its
// own here, so nearestWorkspaceAnchor's walk falls all the way through both
// to the panel root — pinning that a chat's OWN WorkspaceID pointer is never
// enough to make it an anchor on its own (see nearestWorkspaceAnchor's doc);
// only a row that IS its own Node{Kind:workspace} counts. Caught live before
// this simplification: this was refused with ErrCrossContext, because
// nearestWorkspaceAnchor used to stop at the FIRST row carrying a
// WorkspaceID rather than the row that actually OWNED it.
func TestMove_AllowsAFolderIntoAPlainChatSiblingAtTheSameRoot(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedChat(chats, "owner", 1)
	seedChat(chats, "sibling", 2)
	folder, _, err := uc.Create(context.Background(), tree.CreateInput{RepoID: repoID, Name: "folder"})
	require.NoError(t, err)

	placed, _, err := uc.Move(context.Background(), folder.ID, tree.MoveInput{ParentID: name("sibling")})
	require.NoError(t, err)
	assert.Equal(t, "sibling", placed.ParentID)
}

// 2026-09-08 sidebar-placement-unification Task 8's own additions: folder
// discovery/densify against an UN-NODED branch container ("branch-1", a raw
// Chat fixture that never itself goes through placeChat/Node — the golden
// rule's own fallback, per validate.go's resolveRow/parentOf) has to see
// MULTIPLE Node-backed siblings sharing it, not just the one
// TestMove_AllowsAMoveWithinTheSameBranchContext already proves — this is
// the direct test for mergeForest's "seed every known chat id" BFS fix
// (home_forest.go): before it, a folder filed under a Chat-only container
// with no Node row of its own was invisible to any OTHER folder's own
// densify/discovery pass sharing that same container.
func TestMove_DensifiesAgainstASiblingFolderSharingAnUnNodedBranchContainer(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedChat(chats, "branch-1", 1)
	first, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "first",
	})
	require.NoError(t, err)
	require.Equal(t, 0, first.Order)

	second, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "second",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, second.Order, "the second folder must see the first as an occupied slot")

	placed, shifted, err := uc.Move(context.Background(), second.ID, tree.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	require.Len(t, shifted, 1)
	assert.Equal(t, first.ID, shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order, "the sibling folder correctly shifted too")
}

// The repo-scoped mirror of project.go's cross-project repo-phantom scoping
// (SDD review Critical 2/fix round 3, home_folder_test.go's own
// TestRegression_PlaceChat_Home_DoesNotCorruptAnotherProjectsRepoSharingTheBareRoot):
// a repo-scoped chat's OWN placement must never include ANOTHER repo's
// Node{Kind:repo} phantom as a sibling at all — a repo is never a
// legitimate sibling INSIDE a repo's own internal tree (see mergeForest's
// own doc, home_forest.go) — so densifying a chat at this repo's bare
// internal root must leave every repo phantom completely untouched,
// regardless of which project or repo it belongs to.
func TestPlaceChat_NeverDensifiesAgainstAnyRepoPhantom(t *testing.T) {
	chats, nodes, gitStatus, uc := newHomeUsecaseWithGitStatus(t)
	gitStatus.SetRepo(workspaceID, repoID)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID})
	nodes.Rows = []domain.Node{
		{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0}, // this repo's OWN phantom
		{ID: "repo-2", Kind: domain.NodeKindRepo, Order: 1}, // an unrelated repo's
	}

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "repo-1").Order, "no repo phantom is EVER a sibling inside a repo's own tree")
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-2").Order)
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "repo-1", w.ID, "a repo phantom must never be WRITTEN by a repo-internal densify")
		assert.NotEqual(t, "repo-2", w.ID)
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "repo-1", w.ID)
		assert.NotEqual(t, "repo-2", w.ID)
	}
}

// PlaceChat makes the identical refusal for a CHAT's own move: a thread below
// the one being dragged is part of the subtree it takes, so it blocks the move
// just as a working chat blocks its own.
func TestPlaceChat_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedChat(chats, "other", 3)
	work.Set("c2", true)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

// TestPlaceChat_SecondMoveOfAChatUnreachableThroughItsBranchParent pins the
// live regression this fix closes (2026-09-09, caught live: "reparent a
// chat" — previously routine — started failing with "create node: exists").
//
// "branch-1" is a LOCKED branch's own owning-chat row: placed through the
// pre-Node placeOwningRow path, so — unlike an ordinary chat — it carries NO
// Node row of its own for mergeForest's BFS to walk THROUGH (mergeHomeNode's
// own doc). "fork-chat" is filed under it, and is the SOLE member of its own
// workspace, so workspaceSnapshotAround's own ListByWorkspace read never
// discovers "branch-1" as a sibling to walk from either. The result: NO walk,
// run any number of times, will ever discover "fork-chat"'s own Node row —
// even though, exactly as here, one already exists from an earlier move.
//
// writeHomeNode used to trust "not discovered by this walk" as proof the row
// was new and mint it again via Nodes.Create, which asynx correctly refuses
// (Validate: "current != nil") — the toast a live second reparent produced.
// This pins that a SECOND move of such a chat now succeeds, writing through
// SetPlacement rather than attempting a second Create.
func TestPlaceChat_SecondMoveOfAChatUnreachableThroughItsBranchParent(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-chat", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork", ParentID: "branch-1"},
	)
	gitStatus.SetRepo("ws-fork", repoID)
	gitStatus.SetRepo("ws-branch-1", repoID)
	// Already Node-backed from an earlier, successful move — the live state
	// the SECOND move actually found.
	nodes.Rows = append(nodes.Rows, domain.Node{
		ID: "fork-chat", Kind: domain.NodeKindChat, ParentID: "branch-1", Order: 0,
	})

	_, _, err := uc.PlaceChat(context.Background(), "ws-fork", "fork-chat", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err,
		"a chat already Node-backed must never be re-Created just because this walk could not reach it")

	assert.Equal(t, "branch-1", nodeRowFor(t, nodes, "fork-chat").ParentID)
}

// TestRegression_PlaceChat_DensifiesAgainstOtherForksUnderTheSameLockedBranch
// pins a second, distinct bug the SAME "fork's own workspace has no
// siblings in it" gap (mergeHomeNode's own doc) causes: workspaceSnapshot-
// Around's ListByWorkspace read for "fork-c" returns ONLY fork-c (each fork
// owns its own private workspace, spec §2.4), and before this fix
// mergeHomeNode's Chat case corrected an ALREADY-known row in place but
// never appended one it discovered fresh — unlike its own Folder case
// immediately above it, which always has. mergeForest's BFS still WALKED
// through "branch-1" and found fork-a/fork-b, but silently dropped both from
// the snapshot, leaving densify a container of one. Caught live: reordering
// a fork inside a locked branch always wrote order 0, never the position
// actually dragged to, no matter what index was requested.
func TestRegression_PlaceChat_DensifiesAgainstOtherForksUnderTheSameLockedBranch(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-a", ParentID: "branch-1"},
		domain.Chat{ID: "fork-b", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-b", ParentID: "branch-1"},
		domain.Chat{ID: "fork-c", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-c", ParentID: "branch-1"},
	)
	gitStatus.SetRepo("ws-fork-a", repoID)
	gitStatus.SetRepo("ws-fork-b", repoID)
	gitStatus.SetRepo("ws-fork-c", repoID)
	gitStatus.SetRepo("ws-branch-1", repoID)
	nodes.Rows = []domain.Node{
		{ID: "fork-a", Kind: domain.NodeKindChat, ParentID: "branch-1", Order: 0},
		{ID: "fork-b", Kind: domain.NodeKindChat, ParentID: "branch-1", Order: 1},
		{ID: "fork-c", Kind: domain.NodeKindChat, ParentID: "branch-1", Order: 2},
	}

	// fork-c's own snapshot is read off ws-fork-c, which names only fork-c —
	// the bug's exact trigger. Target 1 (between fork-a and fork-b) can only
	// land correctly if the walk's OTHER discoveries survive into the plan.
	_, _, err := uc.PlaceChat(context.Background(), "ws-fork-c", "fork-c", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 1, nodeRowFor(t, nodes, "fork-c").Order,
		"the requested index must survive, not collapse to 0 for lack of a known container size")
	assert.Equal(t, 0, nodeRowFor(t, nodes, "fork-a").Order, "untouched: still first")
	assert.Equal(t, 2, nodeRowFor(t, nodes, "fork-b").Order, "pushed down to make room")
}
