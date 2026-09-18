package project_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The project-home level is ONE list of repo headers and home chats/folders.
// These pin the repo-drag writer against the same member set the sidebar
// renders: nothing of kind workspace (a locked branch's anchor shares the ""
// root but is drawn inside its repo), no other project's folders, and every
// home chat — Node-backed or not, minus the hidden home owner.

func newHomeLevelFixture(t *testing.T) (*mocks.RepositoryStore, *mocks.NodePlacements, *mocks.AgentChatPlacements, *mocks.FolderStore, project.Usecase) {
	t.Helper()
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	workspaces := mocks.NewWorkspacePlacements()
	workspaces.Rows = []domain.Workspace{
		{ID: "home-ws-A", ProjectID: "pA", Kind: domain.WorkspaceKindHome},
		{ID: "home-ws-B", ProjectID: "pB", Kind: domain.WorkspaceKindHome},
	}
	homeChats := mocks.NewAgentChatPlacements()
	uc := project.New(mocks.NewProjectStore(), repos, workspaces, folders, nodes, homeChats, nil)
	require.NoError(t, repos.Save(context.Background(), domain.Repository{ID: "repo-A", ProjectID: "pA"}))
	return repos, nodes, homeChats, folders, uc
}

func TestRegression_UpdateRepo_NeverRenumbersAWorkspaceAnchorSharingTheRoot(t *testing.T) {
	_, nodes, homeChats, _, uc := newHomeLevelFixture(t)
	homeChats.Rows = []domain.Chat{
		{ID: "owner", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, OwnsWorkspace: true},
		{ID: "c1", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(1, 0)},
	}
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "locked-main", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
	}

	_, err := uc.UpdateRepo(context.Background(), "repo-A", project.RepoUpdate{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRow(t, nodes, "repo-A").Order)
	assert.Equal(t, 0, nodeRow(t, nodes, "locked-main").Order, "a locked branch's anchor is not a top-level row")
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "locked-main", w.ID, "a workspace anchor must never be written by a repo drag")
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "locked-main", w.ID, "a workspace anchor must never be written by a repo drag")
	}
}

// A home chat created at the root before Node rows existed has only its
// frozen Chat.Order (0). The repo writer must still count it — and mint its
// Node at the slot it lands on — or a repo can never be placed past it.
func TestRegression_UpdateRepo_PassesANodelessHomeChat(t *testing.T) {
	_, nodes, homeChats, _, uc := newHomeLevelFixture(t)
	homeChats.Rows = []domain.Chat{
		{ID: "owner", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, OwnsWorkspace: true},
		{ID: "c1", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(1, 0)},
		{ID: "c2", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(2, 0)},
	}
	nodes.Rows = []domain.Node{{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0}}

	_, err := uc.UpdateRepo(context.Background(), "repo-A", project.RepoUpdate{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "c1").Order, "c1 keeps the front")
	assert.Equal(t, 1, nodeRow(t, nodes, "repo-A").Order, "the repo sits between the two chats")
	assert.Equal(t, 2, nodeRow(t, nodes, "c2").Order, "c2 is pushed past the repo")
	for _, n := range nodes.Rows {
		assert.NotEqual(t, "owner", n.ID, "the home owner chat is not a row and never gets a slot")
	}
}

func TestRegression_UpdateRepo_IgnoresAnotherProjectsHomeFolder(t *testing.T) {
	_, nodes, _, folders, uc := newHomeLevelFixture(t)
	folders.Saved = []domain.Folder{
		{ID: "folder-A", RepoID: "", HomeID: "home-ws-A"},
		{ID: "folder-B", RepoID: "", HomeID: "home-ws-B"},
		{ID: "repo-folder", RepoID: "repo-A"},
	}
	nodes.Rows = []domain.Node{
		{ID: "folder-A", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "folder-B", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "repo-folder", Kind: domain.NodeKindFolder, Order: 0},
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 1},
	}

	_, err := uc.UpdateRepo(context.Background(), "repo-A", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "repo-A").Order)
	assert.Equal(t, 1, nodeRow(t, nodes, "folder-A").Order)
	assert.Equal(t, 0, nodeRow(t, nodes, "folder-B").Order, "project B's home folder is not a sibling")
	assert.Equal(t, 0, nodeRow(t, nodes, "repo-folder").Order, "a repo-internal folder is not a sibling")
}
