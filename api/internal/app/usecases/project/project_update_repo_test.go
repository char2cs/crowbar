package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestProjectUsecase_UpdateRepo_LoadRepoError covers UpdateRepo's own initial
// lookup failing (a real store error), distinct from the id-not-found case
// covered elsewhere.
func TestProjectUsecase_UpdateRepo_LoadRepoError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	repos.FindByKeyErr = errors.New("db down")

	_, err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("x")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_TargetProjectLookupError covers a cross-project
// move whose TARGET project cannot even be resolved (a store error, not merely
// missing) surfacing before anything is mutated.
func TestProjectUsecase_UpdateRepo_TargetProjectLookupError(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	projects.FindErr = errors.New("db down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.Error(t, err)
	assert.Empty(t, workspaces.Rows, "no workspace relocation attempted when the target project can't be resolved")
	stored, findErr := repos.FindByKey(ctx, "r1")
	require.NoError(t, findErr)
	require.NotNil(t, stored)
	assert.Equal(t, "p1", stored.ProjectID, "the repo must not move on a failed target lookup")
}

// TestProjectUsecase_UpdateRepo_TargetProjectNotFound covers moving a repo to
// a project id that does not exist.
func TestProjectUsecase_UpdateRepo_TargetProjectNotFound(t *testing.T) {
	_, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("nonexistent")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_ListWorkspacesError covers a cross-project move
// whose workspace-listing step (to know what to carry along) fails.
func TestProjectUsecase_UpdateRepo_ListWorkspacesError(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	workspaces.ListErr = errors.New("db down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_RelocateWorkspaceError covers one workspace's
// SetProject write failing mid-move: the repo must not be saved into its new
// project while a workspace it owns is left behind in the old one.
func TestProjectUsecase_UpdateRepo_RelocateWorkspaceError(t *testing.T) {
	projects, repos, workspaces, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	workspaces.Rows = []domain.Workspace{{ID: "w1", ProjectID: "p1", RepoID: "r1"}}
	workspaces.SetErr = errors.New("db down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.Error(t, err)
	stored, findErr := repos.FindByKey(ctx, "r1")
	require.NoError(t, findErr)
	require.NotNil(t, stored)
	assert.Equal(t, "p1", stored.ProjectID, "the repo row must not be saved into the new project")
}

// TestProjectUsecase_UpdateRepo_SaveError covers the repo row itself refusing
// to save after a (possible) project move has already been applied in memory.
func TestProjectUsecase_UpdateRepo_SaveError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	repos.SaveErr = errors.New("db down")

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("renamed")})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_DensifyNewProjectSaveError covers a sibling
// row's renumbering write failing while densifying the repo's (destination)
// project after the update.
func TestProjectUsecase_UpdateRepo_DensifyNewProjectSaveError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Order: 0}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r2", ProjectID: "p1", Order: 1}))
	repos.SaveErrForID = map[string]error{"r2": errors.New("db down")}

	// Moving r1 to slot 1 (after r2) displaces r2 down to slot 0, so densify
	// must renumber (and re-save) the sibling r2, not just r1 itself.
	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Order: index(1)})
	require.Error(t, err)
}

// TestProjectUsecase_UpdateRepo_DensifyOriginProjectSaveError covers the
// SECOND densify pass — the ORIGIN project's list, renumbered after a repo
// moves OUT of it — failing independently of the destination project's own
// (successful) densify.
func TestProjectUsecase_UpdateRepo_DensifyOriginProjectSaveError(t *testing.T) {
	projects, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p2"}))
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Order: 0}))
	// A sibling left behind in the ORIGIN project (p1), which the post-move
	// densify of p1 must renumber.
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r-sibling", ProjectID: "p1", Order: 1}))

	repos.FindWhereFn = func(match domain.Repository) ([]domain.Repository, error) {
		if match.ProjectID == "p1" {
			return nil, errors.New("origin densify boom")
		}
		rows := make([]domain.Repository, 0)
		for _, r := range repos.Saved {
			if r.ProjectID == match.ProjectID {
				rows = append(rows, r)
			}
		}
		return rows, nil
	}

	_, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{ProjectID: name("p2")})
	require.Error(t, err, "a failure densifying the ORIGIN project must surface even though the move itself already saved")
	stored, findErr := repos.FindByKey(ctx, "r1")
	require.NoError(t, findErr)
	require.NotNil(t, stored)
	assert.Equal(t, "p2", stored.ProjectID,
		"the repo's own move already committed before the origin densify ran")
}

// TestProjectUsecase_UpdateRepo_RefetchFailsFallsBackToInMemoryRow covers the
// final re-fetch (done purely to hand back a fresher row) failing or finding
// nothing: UpdateRepo must still return the row it just constructed and saved,
// rather than erroring on a best-effort read.
func TestProjectUsecase_UpdateRepo_RefetchFailsFallsBackToInMemoryRow(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1", Name: "old"}))

	calls := 0
	repos.FindByKeyFn = func(id string) (*domain.Repository, error) {
		calls++
		if calls == 1 {
			// The load at the top of UpdateRepo must still succeed normally.
			for i := range repos.Saved {
				if repos.Saved[i].ID == id {
					row := repos.Saved[i]
					return &row, nil
				}
			}
			return nil, nil
		}
		// The post-save re-fetch fails.
		return nil, errors.New("refetch boom")
	}

	got, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("renamed")})
	require.NoError(t, err, "a failed best-effort re-fetch must not fail the whole update")
	assert.Equal(t, "renamed", got.Name, "the caller still gets back the row UpdateRepo itself just built")
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
