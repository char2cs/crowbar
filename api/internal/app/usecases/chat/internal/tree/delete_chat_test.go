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

func TestDeleteChat_AReHungRowRisesPastEveryTakenParent(t *testing.T) {
	f := newForkedMain(t, worktreeIn("ws-fb"), worktreeIn("ws-fc"))
	f.reaper.Takes["ws-main"] = []string{"ws-fa", "ws-fb"}
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "fa", "ws-fa", "")
	seedWorktreeChat(f.chats, "fb", "ws-fb", "fa")
	seedWorktreeChat(f.chats, "fc", "ws-fc", "fb")
	before := chatRow(t, f.chats, "fc")

	removed, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"main", "fa", "fb"}, removed.Chats)
	fc := chatRow(t, f.chats, "fc")
	assert.Empty(t, fc.ParentID, "feature-c rises past feature-b and feature-a to the root")
	before.ParentID, before.Order = fc.ParentID, fc.Order
	assert.Equal(t, before, fc, "only its place changes")
	assert.NotContains(t, f.chats.Purged, "fc")
	assert.Equal(t, []string{"ws-fc"}, f.rosterIDs())
}

func TestDeleteChat_ATakenRowUnderATakenParentHandsItsChildToTheRoot(t *testing.T) {
	f := newForkedMain(t, worktreeIn("ws-fz"), worktreeIn("ws-x"))
	f.reaper.Takes["ws-main"] = []string{"ws-fa", "ws-fz"}
	seedWorktreeChat(f.chats, "main", "ws-main", "")
	seedWorktreeChat(f.chats, "fz", "ws-fz", "")
	seedWorktreeChat(f.chats, "fa", "ws-fa", "fz")
	seedWorktreeChat(f.chats, "x", "ws-x", "fa")

	_, err := f.uc.DeleteChat(context.Background(), "main", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Empty(t, chatRow(t, f.chats, "x").ParentID)
}

func seedOwner(chats *mocks.AgentChatPlacements, id, wsID string) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: id, Type: domain.ChatTypeChat, WorkspaceID: wsID, OwnsWorkspace: true,
	})
}

func TestDeleteChat_AWorkspacesOwningRowTakesEveryChatRunningInIt(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-fa"))
	seedOwner(chats, "owner", "ws-fa")
	seedWorktreeChat(chats, "perf", "ws-fa", "")
	seedWorktreeChat(chats, "rich", "ws-fa", "")
	seedWorktreeChat(chats, "bubble", "", "rich")
	risk := domain.WorkAtRisk{WorkspaceID: "ws-fa", Branch: "feature/a", UnmergedCommits: 3}
	reaper.Risks = map[string]domain.WorkAtRisk{"ws-fa": risk}

	_, err := uc.DeleteChat(context.Background(), "owner", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Empty(t, chats.Purged, "the refusal comes before any chat goes")
	assert.Len(t, roster.Rows, 1)

	removed, err := uc.DeleteChat(context.Background(), "owner", domain.DiscardWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"perf", "rich", "bubble"}, removed.Taken)
	assert.ElementsMatch(t, []string{"owner", "perf", "rich", "bubble"}, chats.Purged)
	assert.Equal(t, []string{"ws-fa"}, reaper.Reaped)
	assert.Empty(t, roster.Rows, "no fresh owner keeps the workspace alive")
}

func TestDeleteChat_ANonOwningChatInAWorkspaceTakesNoNeighbour(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-fa"))
	seedOwner(chats, "owner", "ws-fa")
	seedWorktreeChat(chats, "perf", "ws-fa", "")
	seedWorktreeChat(chats, "rich", "ws-fa", "")

	removed, err := uc.DeleteChat(context.Background(), "perf", domain.DiscardWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"perf"}, removed.Chats)
	assert.Empty(t, removed.Taken)
	assert.Empty(t, reaper.Reaped)
	assert.Len(t, roster.Rows, 1)
}

func TestDeleteChat_TheMainCheckoutsOwningRowTakesNoNeighbour(t *testing.T) {
	chats := mocks.NewAgentChatPlacements()
	roster := mocks.NewAgentWorkspaceRoster()
	roster.Rows = []domain.Workspace{worktreeIn("ws-main")}
	reaper := mocks.NewAgentWorkspaceReaperOver(roster)
	status := mocks.NewAgentWorkspaceGitStatus()
	status.SetRepo("ws-main", repoID)
	status.SetDefault(repoID, "ws-main")
	uc := tree.New(chats, chats, newBusyChats(), status, reaper,
		mocks.NewAgentWorkspaceHolders(chats), mocks.NewFolderStore(), mocks.NewNodePlacements())
	seedOwner(chats, "main", "ws-main")
	seedWorktreeChat(chats, "perf", "ws-main", "")

	removed, err := uc.DeleteChat(context.Background(), "main", domain.DiscardWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"main"}, removed.Chats)
	assert.Empty(t, reaper.Reaped)
}
