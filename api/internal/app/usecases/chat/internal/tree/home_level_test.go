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

// The project-home level as the chat writer counts it: the project's repos,
// its home chats and its home folders — never a locked branch's anchor (drawn
// inside its repo), never the hidden home owner, never another project's
// rows. And a root create lands at the next slot with a Node row, so the
// level is dense from birth.

func newHomeUsecaseFull(
	t *testing.T,
) (*mocks.AgentChatPlacements, *mocks.FolderStore, *mocks.NodePlacements, *mocks.AgentWorkspaceGitStatus, tree.Usecase) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	chats.Nodes = nodes
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, inflight.NewWork(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, folders, nodes, gitStatus, uc
}

func writesTo(nodes *mocks.NodePlacements, id string) bool {
	for _, w := range nodes.Ordered {
		if w.ID == id {
			return true
		}
	}
	for _, w := range nodes.Placed {
		if w.ID == id {
			return true
		}
	}
	return false
}

func TestRegression_CreateChat_AtRootMintsANodeAtTheNextSlot(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
	}
	chats.NextID = "c-new"

	chatID, _, err := uc.CreateChat(context.Background(), homeWorkspaceID, "claude", "", tree.WorktreeSpec{}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	n := nodeRowFor(t, nodes, "c-new")
	assert.Equal(t, "", n.ParentID)
	assert.Equal(t, 2, n.Order, "a root create appends at the next slot")
	assert.False(t, writesTo(nodes, "repo-A"), "an already dense level is not renumbered by a create")
	assert.NotEmpty(t, chats.Started, "the runner still starts once the row is placed")
}

func TestRegression_PlaceChat_HomeLevelNeverCountsAWorkspaceAnchor(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	gitStatus.SetRepo("locked-main", "repo-A")
	gitStatus.SetBranch("locked-main", true)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(2, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "locked-main", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
		{ID: "c2", Kind: domain.NodeKindChat, Order: 2},
	}

	// Visible: [repo-A, c1, c2]. Drag c2 to the END (index 2 = stays) then
	// c1 to the end: [repo-A, c2, c1].
	_, _, err := uc.PlaceChat(context.Background(), homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(2)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "repo-A").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "c2").Order)
	assert.Equal(t, 2, nodeRowFor(t, nodes, "c1").Order)
	assert.Equal(t, 0, nodeRowFor(t, nodes, "locked-main").Order)
	assert.False(t, writesTo(nodes, "locked-main"), "a locked branch's anchor is never a home-level sibling")
}

func TestRegression_PlaceChat_HomeLevelSkipsTheHomeOwner(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, OwnsWorkspace: true},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
	}

	_, _, err := uc.PlaceChat(context.Background(), homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-A").Order)
	for _, n := range nodes.Rows {
		assert.NotEqual(t, "owner", n.ID, "the home owner takes no slot")
	}
	for _, w := range chats.Ordered {
		assert.NotEqual(t, "owner", w.ChatID, "the home owner is never renumbered")
	}
}

func TestRegression_PlaceChat_HomeLevelIgnoresOtherProjectsAndRepoFolders(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	folders.Saved = []domain.Folder{
		{ID: "home-folder", RepoID: "", HomeID: homeWorkspaceID},
		{ID: "other-home-folder", RepoID: "", HomeID: "home-ws-2"},
		{ID: "repo-folder", RepoID: "repo-A"},
	}
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
		domain.Chat{ID: "other-chat", Type: domain.ChatTypeChat, WorkspaceID: "home-ws-2", CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "home-folder", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "other-home-folder", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "repo-folder", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "other-chat", Kind: domain.NodeKindChat, Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
	}

	_, _, err := uc.PlaceChat(context.Background(), homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "home-folder").Order)
	for _, id := range []string{"other-home-folder", "repo-folder", "other-chat"} {
		assert.Equal(t, 0, nodeRowFor(t, nodes, id).Order, id)
		assert.False(t, writesTo(nodes, id), "%s is not a sibling of this project's home level", id)
	}
}

// The reverse leak: a move INSIDE a repo (a repo-root folder past a locked
// branch) must never renumber the project-home rows sharing the bare root.
func TestRegression_MoveRepoFolder_NeverRenumbersHomeRows(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo("locked-main", "repo-A")
	gitStatus.SetBranch("locked-main", true)
	gitStatus.SetRepo("locked-other", "repo-B")
	gitStatus.SetBranch("locked-other", true)
	folders.Saved = []domain.Folder{
		{ID: "F", RepoID: "repo-A"},
		{ID: "home-folder", RepoID: "", HomeID: homeWorkspaceID},
	}
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "home-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "locked-main", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "F", Kind: domain.NodeKindFolder, Order: 1},
		{ID: "locked-other", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "home-chat", Kind: domain.NodeKindChat, Order: 1},
		{ID: "home-folder", Kind: domain.NodeKindFolder, Order: 2},
	}

	_, _, err := uc.Move(context.Background(), "F", tree.MoveInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "F").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "locked-main").Order)
	for _, id := range []string{"locked-other", "repo-A", "home-chat", "home-folder"} {
		assert.False(t, writesTo(nodes, id), "%s is not a sibling inside repo-A", id)
	}
}

// A home chat dragged past a repo header renumbers that repo's Node row, but
// the chat placement path announced folders and the chat only — the repo's
// Node write has no hub projection of its own, so the sidebar kept the stale
// repo order (tied against the chat's new one) until a reload.
func TestRegression_PlaceChat_AnnouncesEveryRepoItShifted(t *testing.T) {
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	chats.Nodes = nodes
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A", "repo-B")
	type announced struct {
		id, parent string
		order      int
	}
	var frames []announced
	uc := tree.New(chats, chats, inflight.NewWork(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes,
		tree.WithRepoAnnouncer(func(_ context.Context, id, parent string, order int) {
			frames = append(frames, announced{id, parent, order})
		}))
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "repo-B", Kind: domain.NodeKindRepo, Order: 1},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 2},
	}

	_, _, err := uc.PlaceChat(context.Background(), homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, []announced{{"repo-A", "", 1}, {"repo-B", "", 2}}, frames)
}
