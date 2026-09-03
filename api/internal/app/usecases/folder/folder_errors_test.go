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
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A store that cannot be read must surface as an error rather than as an empty
// tree the guards would then wave through — a cycle check against rows it could
// not load would approve every move.
func TestGuards_SurfaceAStoreReadError(t *testing.T) {
	boom := errors.New("boom")

	t.Run("create", func(t *testing.T) {
		folders, _, uc := newUsecase(t)
		folders.FindErr = boom
		_, _, err := uc.Create(context.Background(), folder.CreateInput{
			ProjectID: projectID, RepoID: repoID, Name: "spikes",
		})
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("move", func(t *testing.T) {
		folders, _, uc := newUsecase(t)
		folders.Rows = append(folders.Rows, domain.Folder{
			ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
		})
		folders.FindErr = boom
		_, _, err := uc.Move(context.Background(), "f1", folder.MoveInput{Order: index(0)})
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("delete", func(t *testing.T) {
		folders, _, uc := newUsecase(t)
		folders.Rows = append(folders.Rows, domain.Folder{
			ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
		})
		folders.FindErr = boom
		_, err := uc.Delete(context.Background(), "f1")
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("rename", func(t *testing.T) {
		folders, _, uc := newUsecase(t)
		folders.FindErr = boom
		_, err := uc.Rename(context.Background(), "f1", "new")
		assert.ErrorContains(t, err, "boom")
	})

	t.Run("place workspace", func(t *testing.T) {
		_, workspaces, uc := newUsecase(t)
		workspaces.ListErr = boom
		_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1", folder.PlaceInput{})
		assert.ErrorContains(t, err, "boom")
	})
}

func TestRename_SurfacesASaveError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "old",
	})
	folders.SaveErr = errors.New("disk full")

	_, err := uc.Rename(context.Background(), "f1", "new")
	assert.ErrorContains(t, err, "disk full")
}

func TestMove_SurfacesASaveError(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	folders.SaveErr = errors.New("disk full")

	_, _, err = uc.Move(ctx, created.ID, folder.MoveInput{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}

func TestDelete_SurfacesADeleteError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	folders.SaveErr = errors.New("disk full")
	inner := domain.Folder{ID: "f2", ProjectID: projectID, RepoID: repoID, ParentID: "f1"}
	folders.Rows = append(folders.Rows, inner)

	_, err := uc.Delete(context.Background(), "f1")
	assert.ErrorContains(t, err, "disk full",
		"a failed reparent write must not be reported as a clean delete")
}

// The cross-repo lookup itself can fail, and the caller has to hear about it
// rather than get a not-found for a folder that may well exist.
func TestPlaceWorkspace_SurfacesAFolderLookupError(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)
	folders.FindErr = errors.New("boom")

	missing := "elsewhere"
	_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &missing})
	assert.ErrorContains(t, err, "boom")
}

func TestPlaceWorkspace_UnknownFolderIsNotFound(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)

	missing := "nope"
	_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &missing})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// Moving a row back to the root it is already at must not be reported as a
// cross-repo edge just because the empty parent resolves to nothing.
func TestMove_ToTheRootIsAlwaysAllowed(t *testing.T) {
	folders, _, uc := newUsecase(t)
	ctx := context.Background()
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})

	moved, _, err := uc.Move(ctx, "f1", folder.MoveInput{ParentID: name("")})
	require.NoError(t, err)
	assert.Equal(t, "", moved.ParentID)
}

// Two rows created in the same instant still have to come out in a fixed order,
// or a level with identical timestamps reshuffles between requests.
func TestSiblingOrder_FallsBackToTheIDWhenTimestampsTie(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	same := time.Unix(42, 0).UTC()
	workspaces.Rows = append(workspaces.Rows,
		domain.Workspace{ID: "b", ProjectID: projectID, RepoID: repoID, CreatedAt: same},
		domain.Workspace{ID: "a", ProjectID: projectID, RepoID: repoID, CreatedAt: same},
	)

	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, created.Order)
	assert.Equal(t, 0, workspaceRow(t, workspaces, "a").Order, "the id breaks the tie, deterministically")
	assert.Equal(t, 1, workspaceRow(t, workspaces, "b").Order)
}

// A move whose target is a row the level does not hold must leave the level
// alone rather than silently appending it.
func TestMove_ReorderingARowThatLeftIsANoOpForTheOldLevel(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "box",
	})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: box.ID, Name: "inner",
	})
	require.NoError(t, err)

	// Move it out and immediately back: both levels stay dense throughout.
	_, _, err = uc.Move(ctx, inner.ID, folder.MoveInput{ParentID: name("")})
	require.NoError(t, err)
	_, _, err = uc.Move(ctx, inner.ID, folder.MoveInput{ParentID: &box.ID})
	require.NoError(t, err)

	assert.Equal(t, 0, orderOf(t, folders, inner.ID))
	assert.Equal(t, 0, workspaceRow(t, workspaces, "w1").Order)
	assert.Equal(t, 1, orderOf(t, folders, box.ID))
}

// The cross-repo classification resolves ONE row by key after the repo-scoped
// list has already succeeded. A failure there must surface, not be reported as
// a missing parent.
func TestCheckContainer_SurfacesTheClassificationError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindByKeyErr = errors.New("row read failed")

	_, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, ParentID: "nope", Name: "spikes",
	})
	assert.ErrorContains(t, err, "row read failed")
}

// The persist loop writes both row kinds. A workspace write that the aggregate
// refuses must abort the whole move rather than leave the level half-renumbered
// with a clean 200.
func TestMove_SurfacesAWorkspacePlacementError(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)
	created, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	workspaces.SetErr = errors.New("aggregate refused")

	_, _, err = uc.Move(ctx, created.ID, folder.MoveInput{Order: index(0)})
	assert.ErrorContains(t, err, "aggregate refused")
}

// Delete has to renumber the level the children LANDED on as well as the one
// they left, and a write failure there must abort rather than report success.
func TestDelete_SurfacesAWorkspacePlacementError(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "box",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, "w1", folder.PlaceInput{FolderID: &box.ID})
	require.NoError(t, err)
	workspaces.SetErr = errors.New("aggregate refused")

	_, err = uc.Delete(ctx, box.ID)
	assert.ErrorContains(t, err, "aggregate refused")
}

// A placement that names the folder the workspace is already in is a pure
// reorder: it must not be treated as an arrival from elsewhere and appended.
func TestPlaceWorkspace_SameFolderIsAPureReorder(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)
	box, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "box",
	})
	require.NoError(t, err)
	for _, id := range []string{"w1", "w2"} {
		_, _, err = uc.PlaceWorkspace(ctx, projectID, repoID, id, folder.PlaceInput{FolderID: &box.ID})
		require.NoError(t, err)
	}

	placed, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "w2",
		folder.PlaceInput{FolderID: &box.ID, Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, 1, workspaceRow(t, workspaces, "w1").Order)
}

// A placement with no order at all leaves the row where it is, so a folder-only
// PATCH never silently reshuffles the level.
func TestPlaceWorkspace_NoOrderKeepsThePosition(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)

	placed, _, err := uc.PlaceWorkspace(ctx, projectID, repoID, "w2", folder.PlaceInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, placed.Order)
	assert.Equal(t, 0, workspaceRow(t, workspaces, "w1").Order)
}

// The cycle walk crosses BOTH edge kinds and has to terminate on an id that is
// neither — a parent pointing at a row this repo does not hold. It answers
// rather than spinning.
func TestMove_WalksPastADanglingParentEdge(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	ctx := context.Background()
	seedWorkspace(workspaces, "w1", 1)
	// A folder whose parent names a row this repo does not hold — the shape a
	// deleted workspace leaves behind.
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "orphan", ProjectID: projectID, RepoID: repoID, ParentID: "gone", Name: "orphan",
	})
	subject, _, err := uc.Create(ctx, folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "subject",
	})
	require.NoError(t, err)

	orphan := "orphan"
	moved, _, err := uc.Move(ctx, subject.ID, folder.MoveInput{ParentID: &orphan})
	require.NoError(t, err, "a dangling ancestor edge is walked past, not spun on")
	assert.Equal(t, "orphan", moved.ParentID)
}

// A rename with a name the folder already has still answers with the row, so an
// idempotent client retry is not an error.
func TestRename_ToTheSameNameIsIdempotent(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes", Order: 2,
	})

	got, err := uc.Rename(context.Background(), "f1", "spikes")
	require.NoError(t, err)
	assert.Equal(t, "spikes", got.Name)
	assert.Equal(t, 2, got.Order)
}

// A folder whose parent is a row this repo does not hold must be reported as
// missing, not silently accepted as a repo-root sibling.
func TestPlaceWorkspace_CrossRepoFolderIsRefusedNotSilentlyDropped(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "far", ProjectID: "p2", RepoID: "r9", Name: "far",
	})

	far := "far"
	_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &far})
	assert.ErrorIs(t, err, folder.ErrFolderCrossRepo)
	assert.ErrorContains(t, err, "p2/r9", "the message names where the folder actually lives")
}

// TestPlaceWorkspace_SurfacesACrossRepoLookupError proves checkFolderTarget
// reports a genuine store failure while resolving a folder id outside the
// current snapshot, rather than treating a read error the same as "not found
// anywhere" (which TestPlaceWorkspace_CrossRepoFolderIsRefusedNotSilentlyDropped
// pins for the found-elsewhere case).
func TestPlaceWorkspace_SurfacesACrossRepoLookupError(t *testing.T) {
	folders, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)
	folders.FindByKeyErr = errors.New("store unavailable")

	far := "far"
	_, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &far})

	assert.ErrorContains(t, err, "store unavailable")
}

// TestPlaceWorkspace_EmptyFolderIDUnfilesToRoot proves resolvePlacement treats
// an explicit empty string as "un-file to the repo root", distinct from a nil
// FolderID (which leaves the workspace's current folder untouched).
func TestPlaceWorkspace_EmptyFolderIDUnfilesToRoot(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)
	created, _, err := uc.Create(context.Background(), folder.CreateInput{
		ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	inFolder := created.ID
	_, _, err = uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &inFolder})
	require.NoError(t, err)

	empty := ""
	placed, _, err := uc.PlaceWorkspace(context.Background(), projectID, repoID, "w1",
		folder.PlaceInput{FolderID: &empty})

	require.NoError(t, err)
	assert.Empty(t, placed.FolderID, "an explicit empty FolderID must un-file the workspace to the repo root")
}

func TestMove_SurfacesASnapshotError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	folders.FindWhereErr = errors.New("scan failed")

	_, _, err := uc.Move(context.Background(), "f1", folder.MoveInput{Order: index(0)})

	assert.ErrorContains(t, err, "scan failed")
}

func TestDelete_SurfacesASnapshotError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	folders.FindWhereErr = errors.New("scan failed")

	_, err := uc.Delete(context.Background(), "f1")

	assert.ErrorContains(t, err, "scan failed")
}

func TestDelete_SurfacesARowDeleteError(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.Folder{
		ID: "f1", ProjectID: projectID, RepoID: repoID, Name: "spikes",
	})
	folders.DeleteErr = errors.New("row locked")

	_, err := uc.Delete(context.Background(), "f1")

	assert.ErrorContains(t, err, "row locked")
}

// TestNextSlot_ReturnsTheNextAvailableSlot pins NextSlot's plumbing: it snapshots
// the repo's tree and asks the plan for the next free position in the given
// container — the only usecase entry point that reads a slot without mutating
// anything.
func TestNextSlot_ReturnsTheNextAvailableSlot(t *testing.T) {
	_, workspaces, uc := newUsecase(t)
	seedWorkspace(workspaces, "w1", 1)
	seedWorkspace(workspaces, "w2", 2)

	slot, err := uc.NextSlot(context.Background(), projectID, repoID, "")

	require.NoError(t, err)
	assert.Equal(t, 2, slot, "two root-level rows already occupy slots 0 and 1")
}
