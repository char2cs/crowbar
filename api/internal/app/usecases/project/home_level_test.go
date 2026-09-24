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

// A home chat filed into a Chats-panel folder from before Node rows has no
// Node and a frozen ParentID naming a folder that no longer exists. The
// sidebar draws it at the top level; the repo writer skipped every chat with
// a non-empty ParentID, so a repo could never be placed past it.
func TestRegression_UpdateRepo_CountsAChatFiledUnderAGhostParent(t *testing.T) {
	_, nodes, homeChats, _, uc := newHomeLevelFixture(t)
	homeChats.Rows = []domain.Chat{
		{ID: "owner", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, OwnsWorkspace: true},
		{ID: "ghost-filed", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, ParentID: "retired-folder", CreatedAt: time.Unix(1, 0)},
		{ID: "c2", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(2, 0)},
	}
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 1},
		{ID: "c2", Kind: domain.NodeKindChat, Order: 2},
	}

	// Drawn [ghost-filed, repo-A, c2]; the repo to the very top.
	_, err := uc.UpdateRepo(context.Background(), "repo-A", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "repo-A").Order)
	assert.Equal(t, 1, nodeRow(t, nodes, "ghost-filed").Order, "the ghost-filed chat is counted at the root and minted there")
	assert.Equal(t, "", nodeRow(t, nodes, "ghost-filed").ParentID)
	assert.Equal(t, 2, nodeRow(t, nodes, "c2").Order)
}

// Filing a repo INTO a home folder closed the gap it left over repo rows
// only: the remaining repo was renumbered against its repo siblings alone
// and collided with the untouched chats and folder sharing the level.
func TestRegression_UpdateRepo_LeavingTheRootDensifiesTheWholeLevel(t *testing.T) {
	repos, nodes, homeChats, folders, uc := newHomeLevelFixture(t)
	require.NoError(t, repos.Save(context.Background(), domain.Repository{ID: "repo-B", ProjectID: "pA"}))
	homeChats.Rows = []domain.Chat{
		{ID: "owner", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, OwnsWorkspace: true},
		{ID: "c1", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(1, 0)},
		{ID: "c2", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat, CreatedAt: time.Unix(2, 0)},
	}
	folders.Saved = []domain.Folder{{ID: "F", RepoID: "", HomeID: "home-ws-A"}}
	nodes.Rows = []domain.Node{
		{ID: "c1", Kind: domain.NodeKindChat, Order: 0},
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 1},
		{ID: "c2", Kind: domain.NodeKindChat, Order: 2},
		{ID: "repo-B", Kind: domain.NodeKindRepo, Order: 3},
		{ID: "F", Kind: domain.NodeKindFolder, Order: 4},
	}

	_, err := uc.UpdateRepo(context.Background(), "repo-A", project.RepoUpdate{FolderID: name("F"), Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, "F", nodeRow(t, nodes, "repo-A").ParentID)
	assert.Equal(t, 0, nodeRow(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRow(t, nodes, "c2").Order, "c2 closes the gap the repo left")
	assert.Equal(t, 2, nodeRow(t, nodes, "repo-B").Order, "repo-B stays after c2")
	assert.Equal(t, 3, nodeRow(t, nodes, "F").Order)
}

// TestRegression_UpdateRepo_SiblingRepoFiledInAFolderIsNotReminted reproduces
// the live "reorder repos: mint <id>: node: create: create node: exists:
// asynx: validation failed" failure: once ANY repo of the project is filed
// inside a home folder, every later root-level repo drag refused, because
// withNodelessRows read "absent from ListByParent("")" as "has no Node row".
func TestRegression_UpdateRepo_SiblingRepoFiledInAFolderIsNotReminted(t *testing.T) {
	repos, nodes, _, _, uc := newHomeLevelFixture(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-B", ProjectID: "pA"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-C", ProjectID: "pA"}))
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "repo-C", Kind: domain.NodeKindRepo, Order: 1},
		{ID: "repo-B", Kind: domain.NodeKindRepo, ParentID: "f1", Order: 0},
	}

	_, err := uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(1)})
	require.NoError(t, err)
	assert.Equal(t, "f1", nodeRow(t, nodes, "repo-B").ParentID,
		"a repo filed in a folder must not be dragged to the root by a sibling's reorder")
	assert.Equal(t, 1, nodeRow(t, nodes, "repo-A").Order)
	assert.Equal(t, 0, nodeRow(t, nodes, "repo-C").Order)
}
