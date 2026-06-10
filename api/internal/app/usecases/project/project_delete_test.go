package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeDeleteProjects struct {
	projects map[string]domain.Project
	deleted  []string
	findErr  error
	delErr   error
}

func (f *fakeDeleteProjects) FindByKey(
	_ context.Context,
	id string,
) (*domain.Project, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	p, ok := f.projects[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (f *fakeDeleteProjects) Delete(
	_ context.Context,
	id string,
) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeDeleteRepos struct {
	repos   []domain.Repository
	deleted []string
	delErr  error
}

func (f *fakeDeleteRepos) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	return f.repos, nil
}

func (f *fakeDeleteRepos) Delete(
	_ context.Context,
	id string,
) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeDeleteWorkspaces struct {
	workspaces []domain.Workspace
	deleted    []string
	delErr     error
}

func (f *fakeDeleteWorkspaces) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return f.workspaces, nil
}

func (f *fakeDeleteWorkspaces) Delete(
	_ context.Context,
	id string,
) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeDeleteGit struct {
	removedWorktrees []string
	deletedBranches  []string
	removeErr        error
}

func (f *fakeDeleteGit) WorktreeRemove(
	_ context.Context,
	_ string,
	worktreePath string,
) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removedWorktrees = append(f.removedWorktrees, worktreePath)
	return nil
}

func (f *fakeDeleteGit) ForceDeleteBranch(
	_ context.Context,
	_ string,
	name string,
) error {
	f.deletedBranches = append(f.deletedBranches, name)
	return nil
}

type deleteFixture struct {
	projects   *fakeDeleteProjects
	repos      *fakeDeleteRepos
	workspaces *fakeDeleteWorkspaces
	git        *fakeDeleteGit
	uc         project.DeleteUsecase
}

func newDeleteFixture(
	t *testing.T,
) *deleteFixture {
	t.Helper()
	f := &deleteFixture{
		projects:   &fakeDeleteProjects{projects: map[string]domain.Project{}},
		repos:      &fakeDeleteRepos{},
		workspaces: &fakeDeleteWorkspaces{},
		git:        &fakeDeleteGit{},
	}
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:   f.projects,
		Repos:      f.repos,
		Workspaces: f.workspaces,
		Git:        f.git,
	})
	return f
}

const deleteRepoPath = "/home/u/proj/repo"

func (f *deleteFixture) seedProject() {
	f.projects.projects["p1"] = domain.Project{ID: "p1", Name: "demo", Path: deleteRepoPath}
	f.repos.repos = append(f.repos.repos, domain.Repository{
		ID:        "r1",
		ProjectID: "p1",
		Path:      deleteRepoPath,
	})
}

func TestProjectDelete_NotFound(t *testing.T) {
	f := newDeleteFixture(t)

	err := f.uc.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestProjectDelete_CascadesRecords_RemovesOnlyCrowbarWorktrees(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	crowbarPath := worktreepath.For(deleteRepoPath, "feature/x")
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath, Locked: true},
		{ID: "w-adopted", RepoID: "r1", ProjectID: "p1", Branch: "spike", WorktreePath: "/home/u/elsewhere/spike"},
		{ID: "w-child", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", WorktreePath: crowbarPath},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.ElementsMatch(t, []string{"w-main", "w-adopted", "w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"r1"}, f.repos.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
	assert.Equal(t, []string{crowbarPath}, f.git.removedWorktrees,
		"only the crowbar-created worktree may be removed from disk")
	assert.Equal(t, []string{"feature/x"}, f.git.deletedBranches)
}

func TestProjectDelete_UnlockedAdoptedMainWorktree_RecordOnly(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath, Locked: false},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.removedWorktrees,
		"the real repository directory must never be removed, even when the adopted workspace is unlocked")
	assert.Empty(t, f.git.deletedBranches)
	assert.Equal(t, []string{"w-main"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

func TestProjectDelete_SkipsOtherProjectsRows(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.repos = append(f.repos.repos, domain.Repository{ID: "r2", ProjectID: "p2", Path: "/other"})
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-other", RepoID: "r2", ProjectID: "p2", Branch: "main", WorktreePath: "/other"},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.workspaces.deleted)
	assert.Equal(t, []string{"r1"}, f.repos.deleted)
}

func TestProjectDelete_WorktreeRemoveFailure_StillDeletesRecords(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.git.removeErr = errors.New("stale worktree")
	crowbarPath := worktreepath.For(deleteRepoPath, "feature/x")
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-child", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", WorktreePath: crowbarPath},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.deletedBranches, "branch delete must not run after a failed worktree remove")
	assert.Equal(t, []string{"w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

func TestProjectDelete_WorkspaceRecordDeleteError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.delErr = errors.New("db down")
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath, Locked: true},
	}

	err := f.uc.Delete(context.Background(), "p1")
	require.Error(t, err)
	assert.Empty(t, f.repos.deleted)
	assert.Empty(t, f.projects.deleted)
}
