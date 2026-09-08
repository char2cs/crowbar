package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// nodeRow returns the Node row a fake NodePlacements holds for id, failing the
// test if there is none — every test below that reads a repo's own position
// seeds the Node fake itself, so a missing row is a fixture bug, not a "not
// found yet" case worth degrading past.
func nodeRow(
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
	t.Fatalf("nodeRow: no Node row for %s", id)
	return domain.Node{}
}

// nodeOrders reads back the persisted Order of each repo's own Node row, so a
// test can assert the whole level rather than one row of it — the Node-based
// analogue of the old repoOrders (which read domain.Repository.Order directly,
// before a repo's position moved onto Node).
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
		nodes.Rows = append(nodes.Rows, domain.Node{ID: id, Kind: domain.NodeKindRepo, Order: i})
	}

	_, err := uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, nodeOrders(t, nodes, "a", "b", "c"))

	// Re-running the identical move must converge on the same sequence, not
	// drift a slot each time.
	_, err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, nodeOrders(t, nodes, "a", "b", "c"))

	// An out-of-range index clamps to the end rather than failing the request:
	// the client's index was computed against a list that may have moved.
	_, err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(99)})
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, nodeOrders(t, nodes, "a", "b", "c"))
}

// TestRegression_UpdateRepo_SingleRepoDragDoesNotClampToZero pins the
// ORIGINAL production bug this whole migration exists to fix, now against
// Node end to end. Caught live: with only ONE repo in a project, the OLD
// densifyRepos (repo-only, Repository.Order) had ZERO other repos to place
// against — reinsert (ordering.go) clamps to len(slots) after removing the
// moved row, and a repo-only view of "how many OTHER repos share this
// folder" is zero the instant a project has a single repo, no matter how
// many home chats/folders sit beside it. So a repo dragged to sit AFTER a
// home chat always snapped straight back to the very front, no matter what
// position the drag actually asked for.
//
// Node.ListByParent now answers the WHOLE sibling space directly — repos,
// chats and folders alike (2026-09-08 sidebar-placement-unification Task 5
// deleted the interim cross-aggregate merge with the chat package once
// chats/folders became Node-backed too) — which is the fix: a repo's Order
// is placed against the SAME sibling space it actually renders in, read from
// one call, no merge step.
func TestRegression_UpdateRepo_SingleRepoDragDoesNotClampToZero(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-1", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "repo-1", Kind: domain.NodeKindRepo},
		{ID: "chat-1", Kind: domain.NodeKindChat, Order: 0},
		{ID: "chat-2", Kind: domain.NodeKindChat, Order: 1},
	}

	// Drag the SOLE repo to sit after both home chats: order 2, not 0.
	_, err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(2)})
	require.NoError(t, err)

	got := nodeRow(t, nodes, "repo-1").Order
	assert.NotEqual(t, 0, got,
		"the ORIGINAL bug: a single repo's drag silently snapped back to order 0")
	assert.Equal(t, 2, got, "the repo must land exactly where the drag asked, among its real siblings")

	// It must STAY there: an identical re-drag must not drift it.
	_, err = uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, nodeRow(t, nodes, "repo-1").Order,
		"the repo's position must be stable across an identical re-drag")
}

// TestUpdateRepo_PlacesAgainstHomeChatsToo pins placeRepoAmongHomeSiblings'
// whole point now that the merge with the chat package is gone (2026-09-08
// sidebar-placement-unification Task 5): a repo's own Node write densifies
// against home CHAT Node rows read from the SAME Node.ListByParent call, no
// separate chat-side surface at all.
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
		nodes.Rows = append(nodes.Rows, domain.Node{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0})
		return nodes, uc
	}

	t.Run("sorts the repo AFTER a home chat when the target says so, not always before it", func(t *testing.T) {
		nodes, uc := newFixture(t)
		nodes.Rows = append(nodes.Rows, domain.Node{ID: "chat-1", Kind: domain.NodeKindChat, Order: 1})
		ctx := context.Background()

		_, err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(1)})
		require.NoError(t, err)

		repoOrder := nodeRow(t, nodes, "repo-1").Order
		chatOrder := nodeRow(t, nodes, "chat-1").Order
		assert.Less(t, chatOrder, repoOrder, "the chat must sort BEFORE the repo")
	})

	t.Run("sorts the repo BEFORE a home chat when the target says so, not always after it", func(t *testing.T) {
		nodes, uc := newFixture(t)
		nodes.Rows = append(nodes.Rows, domain.Node{ID: "chat-1", Kind: domain.NodeKindChat, Order: 0})
		ctx := context.Background()

		_, err := uc.UpdateRepo(ctx, "repo-1", project.RepoUpdate{Order: index(0)})
		require.NoError(t, err)

		repoOrder := nodeRow(t, nodes, "repo-1").Order
		chatOrder := nodeRow(t, nodes, "chat-1").Order
		assert.Less(t, repoOrder, chatOrder, "the repo must sort BEFORE the chat")
	})

	t.Run("placing a repo INTO a home folder positions it against that folder’s real children", func(t *testing.T) {
		repos := mocks.NewRepositoryStore()
		nodes := mocks.NewNodePlacements()
		folders := mocks.NewFolderStore()
		folders.Saved = append(folders.Saved, domain.Folder{ID: "home-folder-1", RepoID: ""})
		uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-1", ProjectID: "p1"}))
		nodes.Rows = []domain.Node{
			{ID: "repo-1", Kind: domain.NodeKindRepo, Order: 0},
			{ID: "home-folder-1", Kind: domain.NodeKindFolder, Order: 0},
			{ID: "chat-in-folder", Kind: domain.NodeKindChat, ParentID: "home-folder-1", Order: 0},
		}

		_, err := uc.UpdateRepo(ctx, "repo-1",
			project.RepoUpdate{FolderID: name("home-folder-1"), Order: index(1)})
		require.NoError(t, err)

		n := nodeRow(t, nodes, "repo-1")
		assert.Equal(t, "home-folder-1", n.ParentID)
		chatOrder := nodeRow(t, nodes, "chat-in-folder").Order
		assert.Less(t, chatOrder, n.Order, "the folder’s existing child must sort BEFORE the repo")
	})
}

// TestRegression_UpdateRepo_BareRootReorderDoesNotCorruptAnotherProjectsHomeChats
// is the SDD review's Critical 2 fix: domain.Node carries no project id of
// its own, so the bare project-home root (folderID == "") is one literal
// container SHARED by every project's home Node rows. Before this fix,
// placeRepoAmongHomeSiblings included every CHAT-kind Node row sharing that
// container unconditionally — reordering ONE repo in project pA renumbered
// (and WROTE, via Nodes.SetOrder) project pB's own home chat as a side
// effect, corrupting a different project's sidebar the user never touched.
// homeChatIDSet restores the scoping homeContainerChats used to guarantee
// before Task 5 deleted the merge it lived in.
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
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "chat-A1", Kind: domain.NodeKindChat, Order: 1},
		// chat-B1 shares the literal "" root but belongs to a DIFFERENT
		// project — the exact row the pre-fix code leaked into pA's densify.
		{ID: "chat-B1", Kind: domain.NodeKindChat, Order: 2},
	}

	// Drag repo-A past BOTH chats sharing the root -- the reinsert this target
	// forces is what actually shifts chat-B1's dense index in the unfiltered
	// (pre-fix) computation; a smaller target can coincidentally leave its
	// index unchanged even when it was never excluded, which would pass by
	// accident rather than by the fix actually working.
	_, err := uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(2)})
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

// TestUpdateRepo_HomeChatsNotWiredDegradesToTheOldUnscopedBehaviour documents
// the fallback deliberately: a caller that never wires HomeChats (nil) gets
// the SAME posture this whole function had for its first Task-5 version —
// every chat sharing the bare root treated as a sibling, unscoped. Not a
// silent trap: homeChatIDSet's own doc names this explicitly.
func TestUpdateRepo_HomeChatsNotWiredDegradesToTheOldUnscopedBehaviour(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	workspaces := mocks.NewWorkspacePlacements()
	workspaces.Rows = []domain.Workspace{{ID: "home-ws-A", ProjectID: "pA", Kind: domain.WorkspaceKindHome}}
	uc := project.New(mocks.NewProjectStore(), repos, workspaces, mocks.NewFolderStore(), nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "repo-A", ProjectID: "pA"}))
	nodes.Rows = []domain.Node{
		{ID: "repo-A", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "chat-elsewhere", Kind: domain.NodeKindChat, Order: 1},
	}

	_, err := uc.UpdateRepo(ctx, "repo-A", project.RepoUpdate{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRow(t, nodes, "chat-elsewhere").Order,
		"with HomeChats unwired, an unrelated chat sharing the root is still treated as a sibling and renumbered")
}

// The move is what carries the repo's workspaces across. Left behind, they would
// still exist but stop rendering: every hierarchical route and the WS namespace
// are keyed on the workspace's own projectId.
func TestUpdateRepo_ProjectMoveCarriesTheWorkspaces(t *testing.T) {
	projects, repos, workspaces, nodes, uc := newProjectUsecaseWithNodesAndWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "kept", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "r1", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "kept", Kind: domain.NodeKindRepo, Order: 1},
	}
	workspaces.Rows = []domain.Workspace{
		{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		{ID: "w2", ProjectID: "p1", RepoID: "r1"},
		{ID: "other", ProjectID: "p1", RepoID: "kept"},
	}

	got, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.NoError(t, err)
	assert.Equal(t, "p2", got.ProjectID)

	moved, err := workspaces.ListInRepo(ctx, "p2", "r1")
	require.NoError(t, err)
	require.Len(t, moved, 2, "every workspace under the repo follows it")

	stayed, err := workspaces.ListInRepo(ctx, "p1", "kept")
	require.NoError(t, err)
	require.Len(t, stayed, 1, "a sibling repo's workspaces are untouched")

	// Both levels are renumbered: the one the repo left and the one it joined.
	assert.Equal(t, []int{0, 0}, nodeOrders(t, nodes, "kept", "r1"))
}

func TestUpdateRepo_UnknownProjectIs404(t *testing.T) {
	_, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("nope")})
	assert.ErrorIs(t, err, apperr.ErrNotFound)

	row, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "p1", row.ProjectID, "a refused move leaves the repo where it was")
}

// A move to the project the repo is already in is a no-op, not a pointless
// rewrite of every workspace.
func TestUpdateRepo_SameProjectMovesNothing(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	workspaces.Rows = []domain.Workspace{{ID: "w1", ProjectID: "p1", RepoID: "r1"}}

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p1")})
	require.NoError(t, err)
	assert.Equal(t, "p1", workspaces.Rows[0].ProjectID)
}

// A repo's own entry may be filed into a project-home folder — the feature
// this whole file's FolderID plumbing exists for. It lands at order 0 as the
// only repo in that folder, and a repo left behind at the root is untouched:
// the two containers densify independently.
func TestUpdateRepo_FilesIntoAHomeFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, domain.Folder{ID: "home-folder", RepoID: ""})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "r1", Kind: domain.NodeKindRepo},
		{ID: "r2", Kind: domain.NodeKindRepo},
	}

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("home-folder")})
	require.NoError(t, err)
	got := nodeRow(t, nodes, "r1")
	assert.Equal(t, "home-folder", got.ParentID)
	assert.Equal(t, 0, got.Order, "the only repo filed into this folder")

	stillRoot := nodeRow(t, nodes, "r2")
	assert.Equal(t, "", stillRoot.ParentID)
	assert.Equal(t, 0, stillRoot.Order, "unaffected by a move in a different container")
}

// A repo's own entry is filed in some project's home, never inside its own
// (or any other repo's) internal tree — that folder organises BRANCHES, not
// repos, and letting a repo land there would be a repo containing itself.
func TestUpdateRepo_RefusesARepoInternalFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, domain.Folder{ID: "repo-folder", RepoID: "other-repo"})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("repo-folder")})
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)

	assert.Empty(t, nodes.Placed, "a refused move leaves the repo where it was")
	assert.Empty(t, nodes.Ordered, "a refused move leaves the repo where it was")
}

// A folder id naming a CHAT, not a folder, is refused the same way an
// unknown id is — chats live on a different store entirely now (Folders is
// domain.Folder-only, 2026-09-08 sidebar-placement-unification Task 5), so
// FindByKey answers not-found for either, exactly the same "not a
// project-home folder" refusal TestUpdateRepo_RefusesAnUnknownFolder pins.
func TestUpdateRepo_RefusesANonFolderTarget(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), mocks.NewNodePlacements(), nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("some-chat")})
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestUpdateRepo_RefusesAnUnknownFolder(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), mocks.NewNodePlacements(), nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{FolderID: name("missing")})
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// A repo that LEAVES a folder closes the gap it left behind, exactly as
// leaving a project does.
func TestUpdateRepo_LeavingAFolderDensifiesIt(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	folders := mocks.NewFolderStore()
	folders.Saved = append(folders.Saved, domain.Folder{ID: "home-folder", RepoID: ""})
	uc := project.New(mocks.NewProjectStore(), repos, nil, folders, nodes, nil)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "a", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "b", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "a", Kind: domain.NodeKindRepo, ParentID: "home-folder", Order: 0},
		{ID: "b", Kind: domain.NodeKindRepo, ParentID: "home-folder", Order: 1},
	}

	_, err := uc.UpdateRepo(ctx, "a", project.RepoUpdate{FolderID: name("")})
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
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestUpdateRepo_SurfacesAStoreError(t *testing.T) {
	// The reorder reads the project's repo list to renumber it. A failure there
	// must surface rather than leave the level silently un-densified.
	t.Run("reorder list", func(t *testing.T) {
		_, repos, uc := newProjectUsecase(t)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
		repos.FindErr = errors.New("boom")
		_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Order: index(0)})
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("save", func(t *testing.T) {
		_, repos, uc := newProjectUsecase(t)
		ctx := context.Background()
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
		repos.SaveErr = errors.New("disk full")
		_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("new")})
		assert.ErrorContains(t, err, "disk full")
	})
}

// Without a relocator the move is refused rather than run: committing the repo
// row while its workspaces stay behind is the one outcome worse than not moving.
func TestUpdateRepo_ProjectMoveNeedsARelocator(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	uc := project.New(projects, repos, nil, mocks.NewFolderStore(), mocks.NewNodePlacements(), nil)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	assert.ErrorContains(t, err, "no workspace relocator wired")
}

// The workspace relocation runs BEFORE the repo row is saved, so a failure
// leaves the repo where its workspaces still are rather than the other way round.
func TestUpdateRepo_FailedRelocationLeavesTheRepoPut(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	workspaces.Rows = []domain.Workspace{{ID: "w1", ProjectID: "p1", RepoID: "r1"}}
	workspaces.SetErr = errors.New("aggregate refused")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	assert.ErrorContains(t, err, "aggregate refused")

	row, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "p1", row.ProjectID)
}

func TestUpdateRepo_SurfacesAWorkspaceListError(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	workspaces.ListErr = errors.New("read model down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	assert.ErrorContains(t, err, "read model down")
}

// A PATCH that carries nothing is a no-op that still reports the row's real
// state, which is how the handler answers an empty body.
func TestUpdateRepo_EmptyUpdateIsANoOp(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "widget"}))

	got, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{})
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

// The row is re-read after the densify so the caller broadcasts what was
// actually persisted. A read that comes back empty is a 404, not a zero value
// passed off as the project.
func TestReorder_MissingAfterTheWriteIs404(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "a"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "b"}))
	projects.FindErr = errors.New("row read failed")

	_, err := uc.Reorder(ctx, "b", 0)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// TestReorder_ListStoreError covers Reorder surfacing a failure reading the
// project list it renumbers over, before any row is touched.
func TestReorder_ListStoreError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	projects.FindAllErr = errors.New("db down")

	_, err := uc.Reorder(context.Background(), "a", 0)
	assert.ErrorContains(t, err, "db down")
}

// TestUpdateRepo_LookupStoreError covers the repo lookup at the very top of
// UpdateRepo failing (as opposed to the repo simply not existing, covered by
// TestProjectUsecase_UpdateRepo_NotFound).
func TestUpdateRepo_LookupStoreError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	repos.FindByKeyErr = errors.New("db down")

	_, err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("x")})
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}

// TestUpdateRepo_TargetProjectLookupError covers applyRepoProject surfacing a
// failure resolving the destination project, distinct from that project simply
// not existing (TestUpdateRepo_UnknownProjectIs404 above).
func TestUpdateRepo_TargetProjectLookupError(t *testing.T) {
	projects, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	projects.FindErr = errors.New("db down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}

// TestUpdateRepo_DensifySaveError covers densifyRepos surfacing a failure
// saving a sibling row it renumbers — as opposed to the row being explicitly
// moved, whose own (unrelated) metadata save already succeeded earlier in
// UpdateRepo. A repo's Order now lives on its own Node row, so the sibling's
// failing write is injected on the Node fake (OrderErrForID), not the
// repository store.
func TestUpdateRepo_DensifySaveError(t *testing.T) {
	repos, nodes, uc := newProjectUsecaseWithNodes(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "r1", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "r2", Kind: domain.NodeKindRepo, Order: 1},
	}
	// r2 moves to the front, which pushes r1 back a slot — r1 is the sibling
	// whose densify-write fails.
	nodes.OrderErrForID = map[string]error{"r1": errors.New("disk full")}

	_, err := uc.UpdateRepo(ctx, "r2", project.RepoUpdate{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}

// TestRegression_UpdateRepo_OriginDensifyErrorSurfacesAfterAlreadyCommittedMove
// covers the SECOND densify call UpdateRepo makes on a cross-project move —
// renumbering the project the repo LEFT. The move itself (the repo's own row)
// has already been saved by the time this runs, so a failure here surfaces to
// the caller even though the repo has, in fact, already relocated; nothing
// unwinds that, because the next reconcile of either project's list corrects
// the numbering from what's on disk.
func TestRegression_UpdateRepo_OriginDensifyErrorSurfacesAfterAlreadyCommittedMove(t *testing.T) {
	projects, repos, _, nodes, uc := newProjectUsecaseWithNodesAndWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []domain.Node{
		{ID: "r1", Kind: domain.NodeKindRepo, Order: 0},
		{ID: "r2", Kind: domain.NodeKindRepo, Order: 1},
	}
	// Once r1 leaves p1, r2 is the sole remaining row and must densify from
	// order 1 down to order 0 — that write is the one made to fail.
	nodes.OrderErrForID = map[string]error{"r2": errors.New("disk full")}

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})

	assert.ErrorContains(t, err, "disk full")
	moved, findErr := repos.FindByKey(ctx, "r1")
	require.NoError(t, findErr)
	require.NotNil(t, moved)
	assert.Equal(t, "p2", moved.ProjectID,
		"the repo's own move already committed before the origin densify ran")
}

// repositoryStoreMissingAfterSave is a one-off store.ScopedStore[domain.Repository, string]
// fake: it answers the first FindByKey (UpdateRepo's initial lookup) and every
// FindWhere (the densify passes) normally, but the SECOND FindByKey call (the
// post-save re-fetch UpdateRepo uses to return what was actually persisted)
// comes back not-found — modelling a row removed by something else between the
// save and the re-read.
type repositoryStoreMissingAfterSave struct {
	row           domain.Repository
	findByKeyCall int
}

func (s *repositoryStoreMissingAfterSave) Save(context.Context, domain.Repository) error {
	return nil
}

func (s *repositoryStoreMissingAfterSave) Delete(context.Context, string) error { return nil }

func (s *repositoryStoreMissingAfterSave) FindByKey(
	_ context.Context,
	id string,
) (*domain.Repository, error) {
	s.findByKeyCall++
	if s.findByKeyCall > 1 {
		return nil, nil
	}
	if id != s.row.ID {
		return nil, nil
	}
	row := s.row
	return &row, nil
}

func (s *repositoryStoreMissingAfterSave) FindAll(
	context.Context,
) ([]domain.Repository, error) {
	return []domain.Repository{s.row}, nil
}

func (s *repositoryStoreMissingAfterSave) FindWhere(
	_ context.Context,
	match domain.Repository,
) ([]domain.Repository, error) {
	if match.ProjectID != "" && match.ProjectID != s.row.ProjectID {
		return nil, nil
	}
	return []domain.Repository{s.row}, nil
}

// TestRegression_UpdateRepo_ReturnsInMemoryRowWhenPostSaveRefetchComesBackEmpty
// pins the fallback at the end of UpdateRepo: the row was just saved
// successfully, so a nil/failed re-fetch is not treated as the update having
// failed — the caller gets back what it just wrote instead of an error.
func TestRegression_UpdateRepo_ReturnsInMemoryRowWhenPostSaveRefetchComesBackEmpty(t *testing.T) {
	repos := &repositoryStoreMissingAfterSave{row: domain.Repository{ID: "r1", ProjectID: "p1", Name: "widget"}}
	uc := project.New(mocks.NewProjectStore(), repos, nil, mocks.NewFolderStore(), mocks.NewNodePlacements(), nil)

	got, err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("renamed")})

	require.NoError(t, err, "a vanished post-save re-fetch must not fail an update that already succeeded")
	assert.Equal(t, "renamed", got.Name, "the caller gets back the row it just wrote")
}
