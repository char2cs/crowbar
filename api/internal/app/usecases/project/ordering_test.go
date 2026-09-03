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

// repoOrders reads back the persisted order of each repo id, so a test can
// assert the whole level rather than one row of it.
func repoOrders(
	t *testing.T,
	uc project.Usecase,
	ctx context.Context,
	find func(context.Context, string) (*domain.Repository, error),
	ids ...string,
) []int {
	t.Helper()
	orders := make([]int, 0, len(ids))
	for _, id := range ids {
		row, err := find(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, row, "repo %s must exist", id)
		orders = append(orders, row.Order)
	}
	return orders
}

func TestUpdateRepo_ReorderLeavesTheProjectDense(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, repos.Save(ctx, domain.Repository{ID: id, ProjectID: "p1", Name: id}))
	}

	_, err := uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, repoOrders(t, uc, ctx, repos.FindByKey, "a", "b", "c"))

	// Re-running the identical move must converge on the same sequence, not
	// drift a slot each time.
	_, err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 0}, repoOrders(t, uc, ctx, repos.FindByKey, "a", "b", "c"))

	// An out-of-range index clamps to the end rather than failing the request:
	// the client's index was computed against a list that may have moved.
	_, err = uc.UpdateRepo(ctx, "c", project.RepoUpdate{Order: index(99)})
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2}, repoOrders(t, uc, ctx, repos.FindByKey, "a", "b", "c"))
}

// The move is what carries the repo's workspaces across. Left behind, they would
// still exist but stop rendering: every hierarchical route and the WS namespace
// are keyed on the workspace's own projectId.
func TestUpdateRepo_ProjectMoveCarriesTheWorkspaces(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "kept", ProjectID: "p1"}))
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
	assert.Equal(t, []int{0, 0}, repoOrders(t, uc, ctx, repos.FindByKey, "kept", "r1"))
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
	uc := project.New(projects, repos, nil)
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
// UpdateRepo.
func TestUpdateRepo_DensifySaveError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Order: 0}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1", Order: 1}))
	// r2 moves to the front, which pushes r1 back a slot — r1 is the sibling
	// whose densify-save fails.
	repos.SaveErrForID = map[string]error{"r1": errors.New("disk full")}

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
	projects, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1"}))
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Order: 0}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1", Order: 1}))
	// Once r1 leaves p1, r2 is the sole remaining row and must densify from
	// order 1 down to order 0 — that save is the one made to fail.
	repos.SaveErrForID = map[string]error{"r2": errors.New("disk full")}

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
	uc := project.New(mocks.NewProjectStore(), repos, nil)

	got, err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("renamed")})

	require.NoError(t, err, "a vanished post-save re-fetch must not fail an update that already succeeded")
	assert.Equal(t, "renamed", got.Name, "the caller gets back the row it just wrote")
}
