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
