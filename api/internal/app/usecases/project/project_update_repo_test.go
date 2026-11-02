package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// TestProjectUsecase_UpdateRepo_LoadRepoError covers UpdateRepo's own initial
// lookup failing (a real store error), distinct from the id-not-found case
// covered elsewhere.
func TestProjectUsecase_UpdateRepo_LoadRepoError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	repos.FindByKeyErr = errors.New("db down")

	err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("x")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_SaveError covers the repo row itself refusing
// to save after a (possible) project move has already been applied in memory.
func TestProjectUsecase_UpdateRepo_SaveError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	repos.SaveErr = errors.New("db down")

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("renamed")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_DensifyNewProjectSaveError covers a sibling
// row's renumbering write failing while densifying the repo's (destination)
// project after the update. Order now lives on the repo's own Node row, not
// domain.Repository, so the sibling's failing write is injected on the Node
// fake (OrderErrForID) rather than the repository store.
func TestProjectUsecase_UpdateRepo_DensifyNewProjectSaveError(t *testing.T) {
	repos, nodes, uc := newProjectUsecaseWithNodes(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1"}))
	nodes.Rows = []tree.Node{
		{ID: "r1", Kind: tree.KindRepo, Order: 0},
		{ID: "r2", Kind: tree.KindRepo, Order: 1},
	}
	nodes.OrderErrForID = map[string]error{"r2": errors.New("db down")}

	// Moving r1 to slot 1 (after r2) displaces r2 down to slot 0, so densify
	// must renumber (and re-save) the sibling r2, not just r1 itself.
	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Order: index(1)})
	require.Error(t, err)
}

// TestProjectUsecase_Reorder_ListError covers Reorder surfacing a failure
// listing every project (the sidebar's full sibling space) before any row is
// touched.
func TestProjectUsecase_Reorder_ListError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	projects.FindAllErr = errors.New("db down")

	_, err := uc.Reorder(context.Background(), "p1", 0)
	require.Error(t, err)
}
