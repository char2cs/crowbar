package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestProjectUsecase_UpdateRepo_LoadRepoError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	repos.FindByKeyErr = errors.New("db down")

	err := uc.UpdateRepo(context.Background(), "r1", project.RepoUpdate{Name: name("x")})
	require.Error(t, err)
}

func TestProjectUsecase_UpdateRepo_SaveError(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"}))
	repos.SaveErr = errors.New("db down")

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("renamed")})
	require.Error(t, err)
}

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

	err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Order: index(1)})
	require.Error(t, err)
}

func TestProjectUsecase_UpdateRepo_UpdatesNameAndAvatar(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()

	_ = repos.Save(ctx, domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "widget", AvatarLabel: "W", AvatarColor: "avatar-slate",
	})

	require.NoError(t, uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("Renamed Repo")}))
	got, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "Renamed Repo", got.Name)
	assert.Equal(t, "R", got.AvatarLabel, "the fallback avatar letter tracks the new name")
	assert.NotEmpty(t, got.AvatarColor, "the fallback avatar color is recomputed from the new name")
}

func TestProjectUsecase_UpdateRepo_LeavesThePathSlugAlone(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()

	_ = repos.Save(ctx, domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "widget", PathSlug: "widget",
	})

	require.NoError(t, uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("Renamed Repo")}))

	stored, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "widget", stored.PathSlug, "the persisted slug is not rewritten")
	assert.Equal(t, "Renamed Repo", stored.Name)
}

func TestProjectUsecase_UpdateRepo_NotFound(t *testing.T) {
	_, _, uc := newProjectUsecase(t)
	ctx := context.Background()

	err := uc.UpdateRepo(ctx, "missing", project.RepoUpdate{Name: name("Whatever")})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}
