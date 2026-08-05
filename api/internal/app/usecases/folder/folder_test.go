package folder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const (
	projectID = "p1"
	repoID    = "r1"
)

func newUsecase(
	t *testing.T,
) (*mocks.FolderStore, *mocks.WorkspacePlacements, folder.Usecase) {
	t.Helper()
	folders := mocks.NewFolderStore()
	workspaces := mocks.NewWorkspacePlacements()
	return folders, workspaces, folder.New(folders, workspaces)
}

// seedWorkspace appends a fork-root workspace at the repo root, created at the
// given second so the creation-order tiebreak is deterministic.
func seedWorkspace(
	workspaces *mocks.WorkspacePlacements,
	id string,
	createdAtSec int64,
) {
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID:        id,
		ProjectID: projectID,
		RepoID:    repoID,
		Branch:    id,
		CreatedAt: time.Unix(createdAtSec, 0).UTC(),
	})
}

// orderOf reads a folder's persisted order.
func orderOf(
	t *testing.T,
	folders *mocks.FolderStore,
	id string,
) int {
	t.Helper()
	row, err := folders.FindByKey(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, row, "folder %s must exist", id)
	return row.Order
}

func workspaceRow(
	t *testing.T,
	workspaces *mocks.WorkspacePlacements,
	id string,
) domain.Workspace {
	t.Helper()
	for _, w := range workspaces.Rows {
		if w.ID == id {
			return w
		}
	}
	t.Fatalf("workspace %s not found", id)
	return domain.Workspace{}
}

func name(v string) *string { return &v }
func index(v int) *int      { return &v }

func TestCreate_AppendsAtTheEndOfTheSiblingSpace(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)

	created, shifted, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Name)
	assert.Equal(t, 2, created.Order, "a new folder lands after the rows already at that level")
	assert.Empty(t, shifted, "the folder is the only folder at this level")

	// The densify runs over the WHOLE sibling space, so the two workspaces that
	// were both sitting on the migration default of 0 come out distinct.
	assert.Equal(t, 0, workspaceRow(t, workspaces, "w1").Order)
	assert.Equal(t, 1, workspaceRow(t, workspaces, "w2").Order)
}

func TestCreate_TrimsAndRefusesABlankName(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()

	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "  spikes  ",
	})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Name)

	_, _, err = uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "   ",
	})
	assert.ErrorIs(t, err, folder.ErrFolderNameRequired)
}

// A folder under a protected branch is the ordinary case, not an edge one: most
// of them will hang off `develop`.
func TestCreate_NestsUnderAProtectedWorkspace(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "locked", ProjectID: projectID, RepoID: repoID,
		Branch: "develop", Status: domain.WorkspaceStatusLocked,
	})

	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "locked", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "locked", created.ParentID)
	assert.Equal(t, 0, orderOf(t, folders, created.ID))
}

func TestCreate_RefusesAnUnknownParent(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "nope", Name: "spikes",
	})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestCreate_RefusesAParentInAnotherRepo(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "other", ProjectID: projectID, RepoID: "r2", Name: "elsewhere",
	})

	_, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "other", Name: "spikes",
	})
	assert.ErrorIs(t, err, folder.ErrFolderCrossRepo)
}

func TestRename_SetsTheNameAndLeavesPlacementAlone(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, ParentID: "w1", Name: "old", Order: 4,
	})

	renamed, err := uc.Rename(context.Background(), "f1", "  new  ")
	require.NoError(t, err)
	assert.Equal(t, "new", renamed.Name)
	assert.Equal(t, "w1", renamed.ParentID, "a rename must not move the folder")
	assert.Equal(t, 4, renamed.Order)
}

func TestRename_NotFound(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, err := uc.Rename(context.Background(), "missing", "new")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// The whole point of the dense index: after any move every row at the level
// holds a distinct 0..n-1 slot, so the next drop index means what it says.
func TestMove_LeavesEveryLevelDenseAndStable(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)
	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)

	// Drag the folder from the end to the front, twice over, and the level must
	// converge on the same dense sequence each time.
	for range 2 {
		_, _, err = uc.Move(ctx, created.ID, folder.MoveInput{Order: index(0)})
		require.NoError(t, err)
		assert.Equal(t, 0, orderOf(t, folders, created.ID))
		assert.Equal(t, 1, workspaceRow(t, workspaces, "w1").Order)
		assert.Equal(t, 2, workspaceRow(t, workspaces, "w2").Order)
	}

	// And back to the end.
	_, _, err = uc.Move(ctx, created.ID, folder.MoveInput{Order: index(99)})
	require.NoError(t, err)
	assert.Equal(t, 2, orderOf(t, folders, created.ID), "an out-of-range index clamps to the end")
	assert.Equal(t, 0, workspaceRow(t, workspaces, "w1").Order)
	assert.Equal(t, 1, workspaceRow(t, workspaces, "w2").Order)
}

func TestMove_DensifiesTheLevelItLeft(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	parent, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "parent",
	})
	require.NoError(t, err)
	a, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: parent.ID, Name: "a",
	})
	require.NoError(t, err)
	b, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: parent.ID, Name: "b",
	})
	require.NoError(t, err)
	require.Equal(t, 1, orderOf(t, folders, b.ID))

	moved, shifted, err := uc.Move(ctx, a.ID, folder.MoveInput{ParentID: name("")})
	require.NoError(t, err)
	assert.Equal(t, "", moved.ParentID)
	assert.Equal(t, 0, orderOf(t, folders, b.ID), "the vacated level closes its gap")

	shiftedIDs := make([]string, 0, len(shifted))
	for _, row := range shifted {
		shiftedIDs = append(shiftedIDs, row.ID)
	}
	assert.Contains(t, shiftedIDs, b.ID,
		"the collateral must be returned so the caller can broadcast it")
}

func TestMove_RefusesACycle(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "outer",
	})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, outer.ID, folder.MoveInput{ParentID: &inner.ID})
	assert.ErrorIs(t, err, folder.ErrFolderCycle, "a folder may not move under its own descendant")

	_, _, err = uc.Move(ctx, outer.ID, folder.MoveInput{ParentID: &outer.ID})
	assert.ErrorIs(t, err, folder.ErrFolderCycle, "a folder may not move onto itself")
}

// A folder can hang off a workspace, and that workspace can be filed inside a
// folder — so the cycle walk has to cross BOTH edge kinds, not just folder ones.
func TestMove_RefusesACycleThroughAWorkspace(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	outer, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "outer",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{FolderID: &outer.ID})
	require.NoError(t, err)
	under, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "w1", Name: "under-w1",
	})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, outer.ID, folder.MoveInput{ParentID: &under.ID})
	assert.ErrorIs(t, err, folder.ErrFolderCycle)
}

func TestMove_RefusesToCarryForkChildrenUnderAnotherWorkspace(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "host", 1)
	seedWorkspace(workspaces, "other", 2)
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "child", ProjectID: projectID, RepoID: repoID, Branch: "child", ParentID: "host",
	})
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "host", Name: "box",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "child",
		folder.PlaceInput{FolderID: &box.ID})
	require.NoError(t, err)

	other := "other"
	_, _, err = uc.Move(ctx, box.ID, folder.MoveInput{ParentID: &other})
	assert.ErrorIs(t, err, folder.ErrForkChainSplit)
}

func TestMove_AllowsAFiledForkChildToStayWithinItsParentSpace(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "host", 1)
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "child", ProjectID: projectID, RepoID: repoID, Branch: "child", ParentID: "host",
	})
	outer, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "host", Name: "outer",
	})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "host", Name: "inner",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "child",
		folder.PlaceInput{FolderID: &inner.ID})
	require.NoError(t, err)

	outerID := outer.ID
	moved, _, err := uc.Move(ctx, inner.ID, folder.MoveInput{ParentID: &outerID})
	require.NoError(t, err)
	assert.Equal(t, outer.ID, moved.ParentID)
	assert.Equal(t, inner.ID, workspaceRow(t, workspaces, "child").FolderID)
}

func TestMove_RefusesACrossRepoParent(t *testing.T) {
	folders, _, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "other", ProjectID: projectID, RepoID: "r2", Name: "elsewhere",
	})

	other := "other"
	_, _, err = uc.Move(ctx, created.ID, folder.MoveInput{ParentID: &other})
	assert.ErrorIs(t, err, folder.ErrFolderCrossRepo)
}

// Delete unfiles; it never destroys. A folder holds no worktrees, so removing
// the workspaces under it would take work the user only meant to move.
func TestDelete_ReparentsChildrenRatherThanCascading(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	outer, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "outer",
	})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{FolderID: &outer.ID})
	require.NoError(t, err)

	reparented, err := uc.Delete(ctx, outer.ID)
	require.NoError(t, err)

	gone, err := folders.FindByKey(ctx, outer.ID)
	require.NoError(t, err)
	assert.Nil(t, gone, "the folder itself is removed")

	survivor, err := folders.FindByKey(ctx, inner.ID)
	require.NoError(t, err)
	require.NotNil(t, survivor, "a child folder is reparented, never deleted")
	assert.Equal(t, "", survivor.ParentID, "children move to the deleted folder's own parent")

	assert.Equal(t, "", workspaceRow(t, workspaces, "w1").FolderID,
		"a workspace filed in the folder surfaces, it does not disappear")

	ids := make([]string, 0, len(reparented))
	for _, row := range reparented {
		ids = append(ids, row.ID)
	}
	assert.Contains(t, ids, inner.ID, "the reparented rows are returned for broadcast")
}

// Deleting a folder under a workspace removes only the organisational edge. A
// fork child surfaces directly under the same parent; its git lineage is never
// rewritten by a folder delete.
func TestDelete_UnderAWorkspaceSurfacesItsForkChildrenUnderThatWorkspace(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "host", 1)
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "filed", ProjectID: projectID, RepoID: repoID, Branch: "filed", ParentID: "host",
	})
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "host", Name: "box",
	})
	require.NoError(t, err)
	nested, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: box.ID, Name: "nested",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "filed", folder.PlaceInput{FolderID: &box.ID})
	require.NoError(t, err)

	_, err = uc.Delete(ctx, box.ID)
	require.NoError(t, err)

	survivor, err := folders.FindByKey(ctx, nested.ID)
	require.NoError(t, err)
	require.NotNil(t, survivor)
	assert.Equal(t, "host", survivor.ParentID, "a folder can hang off a workspace, so it follows")

	filed := workspaceRow(t, workspaces, "filed")
	assert.Equal(t, "", filed.FolderID)
	assert.Equal(t, "host", filed.ParentID, "the fork lineage is never written by a folder delete")
}

func TestDelete_NotFound(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, err := uc.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestPlaceWorkspace_FilesAForkRootAndDensifies(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "box",
	})
	require.NoError(t, err)

	placed, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "w2",
		folder.PlaceInput{FolderID: &box.ID})
	require.NoError(t, err)
	assert.Equal(t, box.ID, placed.FolderID)
	assert.Equal(t, 0, placed.Order, "it is the only row in the folder")
	assert.Equal(t, 0, workspaceRow(t, workspaces, "w1").Order,
		"the level it left closes its gap")
}

// A folder may organise siblings inside one fork-parent space without changing
// their git lineage. This is the ordinary sidebar gesture: feature branches
// under `develop` can be collected into a folder also under `develop`.
func TestPlaceWorkspace_FilesAForkChildUnderItsExistingParent(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "root", 1)
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "child", ProjectID: projectID, RepoID: repoID, Branch: "child", ParentID: "root",
	})
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "root", Name: "box",
	})
	require.NoError(t, err)

	placed, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "child",
		folder.PlaceInput{FolderID: &box.ID})
	require.NoError(t, err)
	assert.Equal(t, box.ID, placed.FolderID)
	assert.Equal(t, "root", placed.ParentID, "filing is organisation, never a rebase")
}

// The invariant the spec names explicitly, enforced server-side rather than
// merely by the UI: a folder under some OTHER workspace cannot carry this fork
// child there without the reparent endpoint moving its lineage first.
func TestPlaceWorkspace_RefusesToSplitAForkChain(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "root", 1)
	seedWorkspace(workspaces, "other", 2)
	workspaces.Rows = append(workspaces.Rows, domain.Workspace{
		ID: "child", ProjectID: projectID, RepoID: repoID, Branch: "child", ParentID: "root",
	})
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "other", Name: "box",
	})
	require.NoError(t, err)

	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "child",
		folder.PlaceInput{FolderID: &box.ID})
	assert.ErrorIs(t, err, folder.ErrForkChainSplit)
	assert.Equal(t, "", workspaceRow(t, workspaces, "child").FolderID)
	assert.Equal(t, "root", workspaceRow(t, workspaces, "child").ParentID)
}

// A forked child may still be REORDERED among its fork siblings; only the folder
// edge is refused.
func TestPlaceWorkspace_ReordersAForkedChildAmongItsSiblings(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "root", 1)
	workspaces.Rows = append(
		workspaces.Rows,
		domain.Workspace{ID: "a", ProjectID: projectID, RepoID: repoID, ParentID: "root", CreatedAt: time.Unix(2, 0).UTC()},
		domain.Workspace{ID: "b", ProjectID: projectID, RepoID: repoID, ParentID: "root", CreatedAt: time.Unix(3, 0).UTC()},
	)

	placed, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "b", folder.PlaceInput{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, "", placed.FolderID)
	assert.Equal(t, 1, workspaceRow(t, workspaces, "a").Order)
}

func TestPlaceWorkspace_RefusesAFolderInAnotherRepo(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "other", ProjectID: projectID, RepoID: "r2", Name: "elsewhere",
	})

	other := "other"
	_, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{FolderID: &other})
	assert.ErrorIs(t, err, folder.ErrFolderCrossRepo)
}

func TestPlaceWorkspace_RefusesAFolderInsideItsOwnSubtree(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	under, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "w1", Name: "under-w1",
	})
	require.NoError(t, err)

	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{FolderID: &under.ID})
	assert.ErrorIs(t, err, folder.ErrFolderCycle)
}

func TestPlaceWorkspace_NotFound(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "missing", folder.PlaceInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// The repo home is lifted out of the tree by the frontend and opened from the
// repo header, so it must not hold a slot in any sibling space — otherwise every
// index the user drops at is off by one.
func TestSiblingSpace_ExcludesTheRepoHomeAndTombstones(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	workspaces.Rows = append(
		workspaces.Rows,
		domain.Workspace{ID: "home", ProjectID: projectID, RepoID: repoID, IsDefault: true},
		domain.Workspace{ID: "dead", ProjectID: projectID, RepoID: repoID, Status: domain.WorkspaceStatusDeleted},
	)
	seedWorkspace(workspaces, "live", 1)

	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, orderOf(t, folders, created.ID),
		"only the one live row counts toward the append index")
}

func TestListInRepo_ScopesToTheRepo(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(
		folders.Rows,
		domain.Folder{ID: "mine", ProjectID: projectID, RepoID: repoID},
		domain.Folder{ID: "theirs", ProjectID: projectID, RepoID: "r2"},
	)

	rows, err := uc.ListInRepo(context.Background(), projectID, repoID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "mine", rows[0].ID)
}

func TestListInRepo_SurfacesAStoreError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindErr = errors.New("boom")

	_, err := uc.ListInRepo(context.Background(), projectID, repoID)
	assert.ErrorContains(t, err, "boom")
}

func TestCreate_SurfacesAWorkspaceListError(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	workspaces.ListErr = errors.New("read model down")

	_, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "read model down")
}

func TestCreate_SurfacesASaveError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.SaveErr = errors.New("disk full")

	_, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "disk full")
}

func TestPlaceWorkspace_SurfacesAPlacementError(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)
	workspaces.SetErr = errors.New("aggregate refused")

	_, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{Order: index(1)})
	assert.ErrorContains(t, err, "aggregate refused")
}

// A caller-supplied id is honoured, so a client can create a folder optimistically
// under an id it already rendered.
func TestCreate_HonoursACallerSuppliedID(t *testing.T) {
	_, _, uc := newUsecase(t)

	created, _, err := uc.Create(context.Background(), folder.CreateInput{
		ID: "chosen", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "chosen", created.ID)
}

// An empty PATCH must be a no-op that still reports the row's real state, which
// is how the handler answers a body carrying only a rename.
func TestMove_WithNoChangeIsANoOp(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)

	got, shifted, err := uc.Move(ctx, created.ID, folder.MoveInput{})
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, 1, orderOf(t, folders, created.ID))
	assert.Empty(t, shifted)
}
