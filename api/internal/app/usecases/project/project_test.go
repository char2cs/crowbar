package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
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

// newProjectUsecaseWithWorkspaces additionally exposes the workspace relocator,
// for the tests that assert a repo move carries its workspaces across.
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
	uc := project.New(projects, repos, workspaces)
	return projects, repos, workspaces, uc
}

// name and index are pointer literals for the partial RepoUpdate fields.
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
	assert.ErrorIs(t, err, apperr.ErrNotFound)
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

func TestProjectUsecase_UpdateRepo_UpdatesNameAndAvatar(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()

	// Seed a repo whose generated avatar is derived from its ORIGINAL name.
	_ = repos.Save(ctx, domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "widget", AvatarLabel: "W", AvatarColor: "avatar-slate",
	})

	got, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("Renamed Repo")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed Repo", got.Name)
	assert.Equal(t, "R", got.AvatarLabel, "the fallback avatar letter tracks the new name")
	assert.NotEmpty(t, got.AvatarColor, "the fallback avatar color is recomputed from the new name")
}

// A rename is a LABEL change. The on-disk slug must not move with it, or every
// worktree already derived under the old slug is stranded while new ones open a
// second tree beside it.
func TestProjectUsecase_UpdateRepo_LeavesThePathSlugAlone(t *testing.T) {
	_, repos, uc := newProjectUsecase(t)
	ctx := context.Background()

	_ = repos.Save(ctx, domain.Repository{
		ID: "r1", ProjectID: "p1", Name: "widget", PathSlug: "widget",
	})

	got, err := uc.UpdateRepo(ctx, "r1", project.RepoUpdate{Name: name("Renamed Repo")})
	require.NoError(t, err)
	assert.Equal(t, "widget", got.PathSlug, "the on-disk identity survives the rename")

	stored, err := repos.FindByKey(ctx, "r1")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "widget", stored.PathSlug, "the persisted slug is not rewritten")
	assert.Equal(t, "Renamed Repo", stored.Name)
}

func TestProjectUsecase_UpdateRepo_NotFound(t *testing.T) {
	_, _, uc := newProjectUsecase(t)
	ctx := context.Background()

	_, err := uc.UpdateRepo(ctx, "missing", project.RepoUpdate{Name: name("Whatever")})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
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

// Update is the partial write behind the sidebar's project rename and its icon.
// Both go through one load-mutate-save so the fields a change must NOT disturb
// travel through untouched — Path above all, which is where the project's repos
// actually live and which a rename must never follow.
func TestProjectUsecase_UpdateRenames(t *testing.T) {
	projects, _, uc := newProjectUsecase(t)
	ctx := context.Background()
	require.NoError(t, projects.Save(ctx, domain.Project{ID: "p1", Name: "old", Path: "/on/disk"}))

	name := "  harbour  "
	got, err := uc.Update(ctx, "p1", project.Update{Name: &name})

	require.NoError(t, err)
	// Trimmed at the edge, so no name reaches the store wearing the whitespace
	// an inline editor makes it easy to leave behind.
	assert.Equal(t, "harbour", got.Name)
	// The rename touches the LABEL only.
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

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	stored, _ := projects.FindByKey(ctx, "p1")
	assert.Equal(t, "harbour", stored.Name, "a refused rename must not have been written")
}

func TestProjectUsecase_UpdateSetsTheIconAsOneChoice(t *testing.T) {
	// Emoji and stored image are one three-state choice, not two independent
	// flags: setting either has to clear the other, or a project shows an emoji
	// it was told to replace with an image.
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
	// New bytes behind a stable URL: the version has to move or the webview's
	// image cache keeps serving the icon that was replaced.
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

	require.ErrorIs(t, err, apperr.ErrNotFound)
}
