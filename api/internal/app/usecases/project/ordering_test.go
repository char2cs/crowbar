package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func withRepoNodes(ids ...string) *mocks.NodePlacements {
	nodes := mocks.NewNodePlacements()
	for _, id := range ids {
		nodes.Rows = append(nodes.Rows, tree.Node{ID: id, Kind: tree.KindRepo})
	}
	return nodes
}

func nodeRow(
	t *testing.T,
	nodes *mocks.NodePlacements,
	id string,
) tree.Node {
	t.Helper()
	for _, n := range nodes.Rows {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("nodeRow: no Node row for %s", id)
	return tree.Node{}
}

func nodeOrders(
	t *testing.T,
	nodes *mocks.NodePlacements,
	ids ...string,
) []int {
	t.Helper()
	orders := make([]int, 0, len(ids))
	for _, id := range ids {
		orders = append(orders, nodeRow(t, nodes, id).Order)
	}
	return orders
}

func TestUpdateRepo_ReorderLeavesTheProjectDense(t *testing.T) {
	repos, nodes, uc := newProjectUsecaseWithNodes(t)
	ctx := context.Background()
	for i, id := range []string{"a", "b", "c"} {
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: id, ProjectID: "p1", Name: id}))
		nodes.Rows = append(nodes.Rows, tree.Node{ID: id, Kind: tree.KindRepo, Order: i})
	}

	err := uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, nodeOrders(t, nodes, "a", "b", "c"))

	err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, nodeOrders(t, nodes, "a", "b", "c"))

	err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(99)})
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, nodeOrders(t, nodes, "a", "b", "c"))
}

func TestRegression_UpdateRepo_SingleRepoDragDoesNotClampToZero(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-1", ProjectID: "p1"}))
	nodes.Rows = []tree.Node{
		{ID: "repo-1", Kind: tree.KindRepo},
		{ID: "chat-1", Kind: tree.KindChat, Order: 0},
		{ID: "chat-2", Kind: tree.KindChat, Order: 1},
	}

	err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(2)})
	require.NoError(t, err)

	got := nodeRow(t, nodes, "repo-1").Order
	assert.NotEqual(t, 0, got,
		"the ORIGINAL bug: a single repo's drag silently snapped back to order 0")
	assert.Equal(t, 2, got, "the repo must land exactly where the drag asked, among its real siblings")

	err = uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, nodeRow(t, nodes, "repo-1").Order,
		"the repo's position must be stable across an identical re-drag")
}

func TestUpdateRepo_PlacesAgainstHomeChatsToo(t *testing.T) {
	newFixture := func(t *testing.T) (
		*mocks.NodePlacements,
		project.Usecase,
	) {
		t.Helper()
		repos := mocks.NewRepositoryStore()
		nodes := mocks.NewNodePlacements()
		uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), nodes, nil)
		require.NoError(t, repos.Save(context.Background(),
			domain.Repository{ID: "repo-1", ProjectID: "p1"}))
		nodes.Rows = append(nodes.Rows, tree.Node{ID: "repo-1", Kind: tree.KindRepo, Order: 0})
		return nodes, uc
	}

	t.Run("sorts the repo AFTER a home chat when the target says so, not always before it", func(t *testing.T) {
		nodes, uc := newFixture(t)
		nodes.Rows = append(nodes.Rows, tree.Node{ID: "chat-1", Kind: tree.KindChat, Order: 1})
		ctx := context.Background()

		err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(1)})
		require.NoError(t, err)

		repoOrder := nodeRow(t, nodes, "repo-1").Order
		chatOrder := nodeRow(t, nodes, "chat-1").Order
		assert.Less(t, chatOrder, repoOrder, "the chat must sort BEFORE the repo")
	})

	t.Run("sorts the repo BEFORE a home chat when the target says so, not always after it", func(t *testing.T) {
		nodes, uc := newFixture(t)
		nodes.Rows = append(nodes.Rows, tree.Node{ID: "chat-1", Kind: tree.KindChat, Order: 0})
		ctx := context.Background()

		err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(0)})
		require.NoError(t, err)

		repoOrder := nodeRow(t, nodes, "repo-1").Order
		chatOrder := nodeRow(t, nodes, "chat-1").Order
		assert.Less(t, repoOrder, chatOrder, "the repo must sort BEFORE the chat")
	})

	t.Run("placing a repo INTO a home folder positions it against that folder’s real children", func(t *testing.T) {
		repos := mocks.NewRepositoryStore()
		nodes := mocks.NewNodePlacements()
		folders := mocks.NewFolderStore()
		folders.Saved = append(folders.Saved, tree.Folder{ID: "home-folder-1", RepoID: ""})
		uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-1", ProjectID: "p1"}))
		nodes.Rows = []tree.Node{
			{ID: "repo-1", Kind: tree.KindRepo, Order: 0},
			{ID: "home-folder-1", Kind: tree.KindFolder, Order: 0},
			{ID: "chat-in-folder", Kind: tree.KindChat, ParentID: "home-folder-1", Order: 0},
		}

		err := uc.UpdateRepo(ctx, "repo-1",
			project.RepoUpdate{FolderID: name("home-folder-1"), Order: index(1)})
		require.NoError(t, err)

		n := nodeRow(t, nodes, "repo-1")
		assert.Equal(t, "home-folder-1", n.ParentID)
		chatOrder := nodeRow(t, nodes, "chat-in-folder").Order
		assert.Less(t, chatOrder, n.Order, "the folder’s existing child must sort BEFORE the repo")
	})
}

func TestRegression_UpdateRepo_BareRootReorderDoesNotCorruptAnotherProjectsHomeChats(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	workspaces := mocks.NewWorkspacePlacements()
	workspaces.Rows = []domain.Workspace{
		{ID: "home-ws-A", ProjectID: "pA", Kind: domain.WorkspaceKindHome},
		{ID: "home-ws-B", ProjectID: "pB", Kind: domain.WorkspaceKindHome},
	}
	homeChats := mocks.NewAgentChatPlacements()
	homeChats.Rows = []domain.Chat{
		{ID: "chat-A1", WorkspaceID: "home-ws-A", Type: domain.ChatTypeChat},
		{ID: "chat-B1", WorkspaceID: "home-ws-B", Type: domain.ChatTypeChat},
	}
	uc := project.New(mocks.NewProjectStore(), repos, workspaces, mocks.NewFolderStore(), nodes, homeChats)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-A", ProjectID: "pA"}))
	nodes.Rows = []tree.Node{
		{ID: "repo-A", Kind: tree.KindRepo, Order: 0},
		{ID: "chat-A1", Kind: tree.KindChat, Order: 1},
		{ID: "chat-B1", Kind: tree.KindChat, Order: 2},
	}

	err := uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(2)})
	require.NoError(t, err)

	assert.Equal(t, 2, nodeRow(t, nodes, "chat-B1").Order,
		"project pB's chat must be untouched by project pA's own repo reorder")
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "chat-B1", w.ID, "project pB's chat must never be WRITTEN by pA's reorder")
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "chat-B1", w.ID, "project pB's chat must never be WRITTEN by pA's reorder")
	}
}

func TestUpdateRepo_ShiftsACollateralHomeChat(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	workspaces := mocks.NewWorkspacePlacements()
	workspaces.Rows = []domain.Workspace{
		{ID: "home-ws-A", ProjectID: "pA", Kind: domain.WorkspaceKindHome},
	}
	uc := project.New(mocks.NewProjectStore(), repos, workspaces, mocks.NewFolderStore(), nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-A", ProjectID: "pA"}))
	nodes.Rows = []tree.Node{
		{ID: "chat-1", Kind: tree.KindChat, Order: 0},
		{ID: "repo-A", Kind: tree.KindRepo, Order: 1},
	}

	require.NoError(t, uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(0)}))

	assert.Equal(t, 0, nodeRow(t, nodes, "repo-A").Order, "the repo itself moved to the top")
	assert.Equal(t, 1, nodeRow(t, nodes, "chat-1").Order, "the chat was pushed down as collateral")
}

func TestUpdateRepo_HomeChatsNotWiredDegradesToTheOldUnscopedBehaviour(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	workspaces := mocks.NewWorkspacePlacements()
	workspaces.Rows = []domain.Workspace{{ID: "home-ws-A", ProjectID: "pA", Kind: domain.WorkspaceKindHome}}
	uc := project.New(mocks.NewProjectStore(), repos, workspaces, mocks.NewFolderStore(), nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-A", ProjectID: "pA"}))
	nodes.Rows = []tree.Node{
		{ID: "repo-A", Kind: tree.KindRepo, Order: 0},
		{ID: "chat-elsewhere", Kind: tree.KindChat, Order: 1},
	}

	err := uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "chat-elsewhere").Order,
		"with HomeChats unwired, an unrelated chat sharing the root is still treated as a sibling and renumbered")
}

func TestUpdateRepo_FilesIntoAHomeFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, tree.Folder{ID: "home-folder", RepoID: ""})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []tree.Node{
		{ID: "r1", Kind: tree.KindRepo},
		{ID: "r2", Kind: tree.KindRepo},
	}

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("home-folder")})
	require.NoError(t, err)
	got := nodeRow(t, nodes, "r1")
	assert.Equal(t, "home-folder", got.ParentID)
	assert.Equal(t, 0, got.Order, "the only repo filed into this folder")

	stillRoot := nodeRow(t, nodes, "r2")
	assert.Equal(t, "", stillRoot.ParentID)
	assert.Equal(t, 0, stillRoot.Order, "unaffected by a move in a different container")
}

func TestUpdateRepo_RefusesARepoInternalFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := withRepoNodes("r1")
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, tree.Folder{ID: "repo-folder", RepoID: "other-repo"})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("repo-folder")})
	require.ErrorIs(t, err, problem.ErrInvalidArgument)

	assert.Empty(t, nodes.Placed, "a refused move leaves the repo where it was")
	assert.Empty(t, nodes.Ordered, "a refused move leaves the repo where it was")
}

func TestUpdateRepo_RefusesANonFolderTarget(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), withRepoNodes("r1"), nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("some-chat")})
	assert.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestUpdateRepo_RefusesAnUnknownFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), withRepoNodes("r1"), nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("missing")})
	assert.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestUpdateRepo_LeavingAFolderDensifiesIt(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, tree.Folder{ID: "home-folder", RepoID: ""})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "a", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "b", ProjectID: "p1"}))
	nodes.Rows = []tree.Node{
		{ID: "a", Kind: tree.KindRepo, ParentID: "home-folder", Order: 0},
		{ID: "b", Kind: tree.KindRepo, ParentID: "home-folder", Order: 1},
	}

	err := uc.UpdateRepo(ctx, "a", project.RepoUpdate{FolderID: name("")})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "b").Order, "b closes the gap a left in the folder")
}

func TestReorder_LeavesTheProjectListDense(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, projects.Save(ctx, domain.Project{ID: id, Name: id}))
	}

	got, err := uc.Reorder(ctx, "c", 0)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Order)

	for id, want := range map[string]int{"a": 1, "b": 2, "c": 0} {
		row, err := projects.FindByKey(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, row)
		assert.Equal(t, want, row.Order, "project %s", id)
	}
}

func TestReorder_UnknownProjectIs404(t *testing.T) {
	_, _, uc := newProjectUsecase(t)

	_, err := uc.Reorder(context.Background(), "missing", 0)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestUpdateRepo_SurfacesAStoreError(t *testing.T) {
	t.Run("reorder list", func(t *testing.T) {
		_, repos, uc := newProjectUsecase(t)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
		repos.FindErr = errors.New("boom")
		err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Order: index(0)})
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("save", func(t *testing.T) {
		_, repos, uc := newProjectUsecase(t)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
		repos.SaveErr = errors.New("disk full")
		err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("new")})
		assert.ErrorContains(t, err, "disk full")
	})
}

func TestUpdateRepo_ProjectMoveIsRefusedAndWritesNothing(t *testing.T) {
	projects, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "old"}))

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2"), Name: name("new")})
	require.ErrorIs(t, err, problem.ErrConflict)
	row, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, domain.Repository{ID: "r1", ProjectID: "p1", Name: "old"}, *row)

	err = uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p1")})
	require.NoError(t, err)
}

func TestUpdateRepo_EmptyUpdateIsANoOp(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "widget"}))

	require.NoError(t, uc.UpdateRepo(ctx, "r1", project.RepoUpdate{}))
	got, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "widget", got.Name)
}

func TestReorder_SurfacesASaveError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "a"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "b"}))
	projects.SaveErr = errors.New("disk full")

	_, err := uc.Reorder(ctx, "b", 0)
	assert.ErrorContains(t, err, "disk full")
}

func TestReorder_MissingAfterTheWriteIs404(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "a"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "b"}))
	projects.FindErr = errors.New("row read failed")

	_, err := uc.Reorder(ctx, "b", 0)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestReorder_ListStoreError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	projects.FindAllErr = errors.New("db down")

	_, err := uc.Reorder(context.Background(), "a", 0)
	assert.ErrorContains(t, err, "db down")
}

func TestUpdateRepo_LookupStoreError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	repos.FindByKeyErr = errors.New("db down")

	err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("x")})
	require.Error(t, err)
	assert.NotErrorIs(t, err, problem.ErrNotFound)
}

func TestUpdateRepo_DensifySaveError(t *testing.T) {
	repos, nodes, uc := newProjectUsecaseWithNodes(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []tree.Node{
		{ID: "r1", Kind: tree.KindRepo, Order: 0},
		{ID: "r2", Kind: tree.KindRepo, Order: 1},
	}
	nodes.OrderErrForID = map[string]error{"r1": errors.New("disk full")}

	err := uc.UpdateRepo(ctx, "r2", project.RepoUpdate{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}
