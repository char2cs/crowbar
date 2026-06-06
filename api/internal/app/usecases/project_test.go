package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newProjectUsecase(
	t *testing.T,
) (
	*mocks.ProjectStore,
	*mocks.RepositoryStore,
	usecases.ProjectUsecase,
) {
	t.Helper()
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	uc := usecases.NewProjectUsecase(projects, repos)
	return projects, repos, uc
}

func TestProjectUsecase_List(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()

	_ = projects.Save(ctx, domain.Project{ID: "p1", Name: "alpha"})
	_ = projects.Save(ctx, domain.Project{ID: "p2", Name: "beta"})

	list, err := uc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestProjectUsecase_Get_Found(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()

	want := domain.Project{ID: "p1", Name: "alpha"}
	_ = projects.Save(ctx, want)

	got, err := uc.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, want.Name, got.Name)
}

func TestProjectUsecase_Get_NotFound(t *testing.T) {
	_, _, uc := newProjectUsecase(t)
	ctx := context.Background()

	_, err := uc.Get(ctx, "missing")
	assert.Error(t, err)
}

func TestProjectUsecase_TouchProjectActivity_HappyPath(t *testing.T) {
	projects, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	now := time.Unix(9000, 0)

	_ = projects.Save(ctx, domain.Project{ID: "p1", Name: "alpha"})
	_ = repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"})

	// no error returned — best-effort
	uc.TouchProjectActivity(ctx, "r1", now)

	got, err := projects.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, now, got.LastActivity)
}

func TestProjectUsecase_TouchProjectActivity_RepoMissing_LogsNotPanics(t *testing.T) {
	_, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	// repo not in store — must not panic, must not return error
	uc.TouchProjectActivity(ctx, "r-gone", time.Now())
}

func TestProjectUsecase_TouchProjectActivity_ProjectMissing_LogsNotPanics(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()

	_ = repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p-gone"})
	uc.TouchProjectActivity(ctx, "r1", time.Now())
}

func TestProjectUsecase_TouchProjectActivity_SaveFails_LogsNotPanics(t *testing.T) {
	projects, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	now := time.Unix(9000, 0)

	_ = projects.Save(ctx, domain.Project{ID: "p1", Name: "alpha"})
	_ = repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"})
	projects.SaveErr = errors.New("db down")

	// must not panic
	uc.TouchProjectActivity(ctx, "r1", now)
}
