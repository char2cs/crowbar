package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// forkedMain is main's chat with feature-a's worktree forked off main in git
// (the reaper's Takes), whatever the chat tree says about where the rows sit.
type forkedMain struct {
	chats  *mocks.AgentChatPlacements
	uc     tree.Usecase
	roster *mocks.AgentWorkspaceRoster
	reaper *mocks.AgentWorkspaceReaper
	work   *busyChats
}

func newForkedMain(t *testing.T, extra ...domain.Workspace) forkedMain {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	roster := mocks.NewAgentWorkspaceRoster()
	roster.Rows = append([]domain.Workspace{worktreeIn("ws-main"), worktreeIn("ws-fa")}, extra...)
	reaper := mocks.NewAgentWorkspaceReaperOver(roster)
	reaper.Takes = map[string][]string{"ws-main": {"ws-fa"}}
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	for _, ws := range roster.Rows {
		gitStatus.SetRepo(ws.ID, ws.RepoID)
	}
	work := newBusyChats()
	uc := tree.New(chats, chats, work, gitStatus, reaper, mocks.NewAgentWorkspaceHolders(chats),
		mocks.NewFolderStore(), mocks.NewNodePlacements())
	return forkedMain{chats: chats, uc: uc, roster: roster, reaper: reaper, work: work}
}

func (f forkedMain) rosterIDs() []string {
	ids := make([]string, 0, len(f.roster.Rows))
	for _, ws := range f.roster.Rows {
		ids = append(ids, ws.ID)
	}
	return ids
}

// A row dragged out of main's subtree still goes when the git cascade takes
// its worktree, and the response names it as taken beyond the selection.
func TestDeleteChat_TakesTheRowOfAWorktreeTheCascadeTakes(t *testing.T) {
	f := newForkedMain(t)
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "fa", "ws-fa", "")

	removed, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "fa"}, removed.Chats)
	assert.Equal(t, []string{"fa"}, removed.Taken)
	assert.Equal(t, []string{"ws-main"}, f.reaper.Reaped)
	assert.Empty(t, f.roster.Rows)
}

// The reap is bounded by git lineage: a chat nested under a taken row in the
// tree, in a worktree the cascade does not take, is re-hung, never reaped.
func TestDeleteChat_ReHangsWhatATakenRowHeldOutsideTheCascade(t *testing.T) {
	f := newForkedMain(t, worktreeIn("ws-fb"))
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "fa", "ws-fa", "")
	seedWorktreeChat(f.chats, "fb", "ws-fb", "fa")

	removed, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "fa"}, removed.Chats)
	assert.Equal(t, []string{"ws-main"}, f.reaper.Reaped)
	assert.Equal(t, []string{"ws-fb"}, f.rosterIDs(), "feature-b's worktree survives")
	assert.Empty(t, chatRow(t, f.chats, "fb").ParentID, "and its row rises to feature-a's parent")
}

// A taken row above the selected chat widens the delete by that row alone:
// nothing is purged twice and a workspace-less sibling under it survives.
func TestDeleteChat_ATakenAncestorTakesOnlyItsOwnRow(t *testing.T) {
	f := newForkedMain(t)
	seedWorktreeChat(f.chats, "fa", "ws-fa", "")
	seedWorktreeChat(f.chats, "main", "ws-main", "fa")
	seedWorktreeChat(f.chats, "sib", "", "fa")

	removed, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "fa"}, removed.Chats)
	assert.ElementsMatch(t, []string{"main", "fa"}, f.chats.Purged, "each row is purged once")
	assert.Empty(t, chatRow(t, f.chats, "sib").ParentID, "the sibling rises, it is not purged")
}

// A working chat in a row the cascade adds refuses the delete before any reap.
func TestDeleteChat_AWorkingChatInATakenRowRefusesTheDelete(t *testing.T) {
	f := newForkedMain(t)
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "fa", "ws-fa", "")
	f.work.Set("fa", true)

	_, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Empty(t, f.reaper.Reaped)
	assert.Empty(t, f.chats.Purged)
}

// A locked root refuses before a reap deeper in its subtree runs.
func TestDeleteChat_ALockedRootRefusesBeforeAnyReap(t *testing.T) {
	f := newForkedMain(t, worktreeIn("ws-child"))
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "child", "ws-child", "main")
	f.reaper.ErrFor["ws-main"] = workspace.ErrWorkspaceLocked

	_, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, workspace.ErrWorkspaceLocked)
	assert.Empty(t, f.reaper.Reaped, "the unlocked child is not reaped first")
	assert.Empty(t, f.chats.Purged)
	assert.Len(t, f.roster.Rows, 3)
}
