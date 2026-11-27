package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func newProjectUsecase(
	t *testing.T,
) (
	*mocks.ProjectStore,
	*mocks.RepositoryStore,
	project.Usecase,
) {
	t.Helper()
	projects, repos, _, uc := newProjectUsecaseWithWorkspaces(t)
	return projects, repos, uc
}

func newProjectUsecaseWithWorkspaces(
	t *testing.T,
) (
	*mocks.ProjectStore,
	*mocks.RepositoryStore,
	*mocks.WorkspacePlacements,
	project.Usecase,
) {
	t.Helper()
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	workspaces := mocks.NewWorkspacePlacements()
	repos.Nodes = mocks.NewNodePlacements()
	uc := project.New(projects, repos, workspaces, mocks.NewFolderStore(), repos.Nodes, nil)
	return projects, repos, workspaces, uc
}

func newProjectUsecaseWithNodes(
	t *testing.T,
) (
	*mocks.RepositoryStore,
	*mocks.NodePlacements,
	project.Usecase,
) {
	t.Helper()
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	uc := project.New(
		mocks.NewProjectStore(), repos, mocks.NewWorkspacePlacements(), mocks.NewFolderStore(), nodes, nil,
	)
	return repos, nodes, uc
}

func name(v string) *string { return &v }
func index(v int) *int      { return &v }

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
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestProjectUsecase_Get_StoreError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	projects.FindErr = errors.New("db down")

	_, err := uc.Get(ctx, "p1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, problem.ErrNotFound, "a lookup failure is not a not-found")
}

func TestProjectUsecase_List_StoreError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	projects.FindAllErr = errors.New("db down")

	_, err := uc.List(ctx)
	assert.ErrorContains(t, err, "db down")
}

func TestProjectUsecase_TouchProjectActivity_HappyPath(t *testing.T) {
	projects, repos, uc := newProjectUsecase(t)
	ctx := context.Background()
	now := time.Unix(9000, 0)

	_ = projects.Save(ctx, domain.Project{ID: "p1", Name: "alpha"})
	_ = repos.Save(ctx, domain.Repository{ID: "r1", ProjectID: "p1"})

	uc.TouchProjectActivity(ctx, "r1", now)

	got, err := projects.FindByKey(ctx, "p1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, now, got.LastActivity)
}

func TestProjectUsecase_TouchProjectActivity_RepoMissing_LogsNotPanics(t *testing.T) {
	_, _, uc := newProjectUsecase(t)
	ctx := context.Background()
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

	uc.TouchProjectActivity(ctx, "r1", now)
}

func TestProjectUsecase_UpdateRenames(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "old", Path: "/on/disk"}))

	name := "  harbour  "
	got, err := uc.Update(ctx, "p1", project.Update{Name: &name})

	require.NoError(t, err)
	assert.Equal(t, "harbour", got.Name)
	assert.Equal(t, "/on/disk", got.Path)

	stored, err := projects.FindByKey(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "harbour", stored.Name)
}

func TestProjectUsecase_UpdateRejectsABlankName(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "harbour"}))

	blank := "   "
	_, err := uc.Update(ctx, "p1", project.Update{Name: &blank})

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	stored, _ := projects.FindByKey(ctx, "p1")
	assert.Equal(t, "harbour", stored.Name, "a refused rename must not have been written")
}

func TestProjectUsecase_UpdateSetsTheIconAsOneChoice(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "harbour", AvatarEmoji: "🦊"}))

	hasIcon, cleared := true, ""
	got, err := uc.Update(ctx, "p1", project.Update{
		AvatarHasIcon:     &hasIcon,
		AvatarEmoji:       &cleared,
		BumpAvatarVersion: true,
	})

	require.NoError(t, err)
	assert.True(t, got.AvatarHasIcon)
	assert.Empty(t, got.AvatarEmoji)
	assert.Equal(t, int64(1), got.AvatarVersion)
}

func TestProjectUsecase_UpdateLeavesUnsetFieldsAlone(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{
		ID: "p1", Name: "harbour", AvatarEmoji: "🦊", AvatarVersion: 3, Order: 2,
	}))

	name := "atlas"
	got, err := uc.Update(ctx, "p1", project.Update{Name: &name})

	require.NoError(t, err)
	assert.Equal(t, "🦊", got.AvatarEmoji, "a rename must not disturb the icon")
	assert.Equal(t, int64(3), got.AvatarVersion, "nor the cache-busting version")
	assert.Equal(t, 2, got.Order, "nor the sidebar order")
}

func TestProjectUsecase_UpdateUnknownProject(t *testing.T) {
	_, _, uc := newProjectUsecase(t)

	name := "nope"
	_, err := uc.Update(context.Background(), "missing", project.Update{Name: &name})

	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestProjectUsecase_UpdateSurfacesAFailedLoad(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	projects.FindErr = errors.New("store is down")

	name := "harbour"
	_, err := uc.Update(context.Background(), "p1", project.Update{Name: &name})

	require.Error(t, err)
}

func TestProjectUsecase_UpdateSurfacesAFailedSave(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "old"}))
	projects.SaveErr = errors.New("disk full")

	name := "harbour"
	_, err := uc.Update(ctx, "p1", project.Update{Name: &name})

	require.Error(t, err)
}

func TestProjectUsecase_Reorder_ListError(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	projects.FindAllErr = errors.New("db down")

	_, err := uc.Reorder(context.Background(), "p1", 0)
	require.Error(t, err)
}
