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
	chats := mocks.NewAgentChatPlacements()
	work := inflight.NewWork()
	roster := mocks.NewAgentWorkspaceRoster()
	// Every fixture in this file is a single-repo world: workspaceID ("ws-1")
	// belongs to repoID ("repo-1"), so a folder created/moved under a plain
	// CHAT parent (which carries workspaceID, not a repo id of its own)
	// resolves to the SAME repo every Create/Move call here already assumes —
	// see domain.Chat.RepoID / checkFolderContainer's golden rule.
	workspaceGitStatus := mocks.NewAgentWorkspaceGitStatus()
	workspaceGitStatus.SetRepo(workspaceID, repoID)
	return chats, tree.New(chats, chats, work, workspaceGitStatus, roster,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats)), work
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
func TestMove_RefusesRootToBranchContext(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc) // "root", a repo-root folder — its own context
	// A workspace-owning row — a locked or unlocked branch is just a chat
	// with a real WorkspaceID from this package's point of view.
	seedChat(chats, "branch-1", 1)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("branch-1")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_RefusesBranchToDifferentBranchContext(t *testing.T) {
	chats := mocks.NewAgentChatPlacements()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"},
		domain.Chat{ID: "branch-2", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"},
	)
	// Both branches resolve to the SAME repo — isolates the finer context
	// check from the coarser repo check TestMove_RefusesAMoveAcrossRepos
	// already covers.
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	gitStatus.SetRepo("ws-1", repoID)
	gitStatus.SetRepo("ws-2", repoID)
	uc2 := tree.New(chats, chats, inflight.NewWork(), gitStatus, mocks.NewAgentWorkspaceRoster(),
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats))
	underBranch1, _, err := uc2.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "notes",
	})
	require.NoError(t, err)

	_, _, err = uc2.Move(context.Background(), underBranch1.ID, tree.MoveInput{ParentID: name("branch-2")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_AllowsAMoveWithinTheSameBranchContext(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedChat(chats, "branch-1", 1)
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
// the folder does not. Caught live: this was refused with ErrCrossContext,
// because nearestWorkspaceAnchor used to stop at the FIRST row carrying a
// WorkspaceID rather than the row that actually OWNS it — "sibling" here is
// an ordinary chat in "owner"'s workspace, never itself an ancestor of the
// folder, exactly like a project's home-owning chat sits outside the very
// tree it roots (rows-from-home.ts) while an ordinary bubble beside it still
// carries that same home workspace id.
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
