package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
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
	findErr error
	delErr  error
}

func (f *fakeDeleteRepos) FindAll(
	_ context.Context,
) ([]domain.Repository, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
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
	listErr    error
	delErr     error
}

func (f *fakeDeleteWorkspaces) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
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
	forceDeleteErr   error
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
	if f.forceDeleteErr != nil {
		return f.forceDeleteErr
	}
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
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return "/home/u/.crowbar", nil },
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
	crowbarPath := "/home/u/.crowbar/projects/github.com/test/repo/workspaces/w-child"
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath, Status: domain.WorkspaceStatusLocked},
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
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath},
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
	crowbarPath := "/home/u/.crowbar/projects/github.com/test/repo/workspaces/w-child"
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-child", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", WorktreePath: crowbarPath},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.deletedBranches, "branch delete must not run after a failed worktree remove")
	assert.Equal(t, []string{"w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

func TestDelete_RemovesProjectDirTree(t *testing.T) {
	// The entity-scoped project directory tree (worktrees + storages + icon
	// under ~/.crowbar/projects/<P>) is rm -rf'd after the GORM rows go.
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	iconPath := filepath.Join(projectDir, "r1", "icon")
	require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0o755))
	require.NoError(t, os.WriteFile(iconPath, []byte("img"), 0o644))

	projects := &fakeDeleteProjects{projects: map[string]domain.Project{
		"p1": {ID: "p1", Name: "demo", Path: "/home/u/proj/repo"},
	}}
	uc := project.NewDelete(project.DeleteDeps{
		Projects:    projects,
		Repos:       &fakeDeleteRepos{},
		Workspaces:  &fakeDeleteWorkspaces{},
		Git:         &fakeDeleteGit{},
		CrowbarHome: func() (string, error) { return home, nil },
	})

	require.NoError(t, uc.Delete(context.Background(), "p1"))

	_, statErr := os.Stat(projectDir)
	assert.True(t, os.IsNotExist(statErr), "the project dir tree must be removed")
	assert.Equal(t, []string{"p1"}, projects.deleted)
}

func TestDelete_NeverTouchesRealRepoPath(t *testing.T) {
	// The user's real repo checkout (an adopted main worktree at repo.Path,
	// living OUTSIDE ~/.crowbar) must survive a project delete. We assert via a
	// RemoveAll seam that only the crowbar projects/<P> dir is ever removed.
	home := "/home/u/.crowbar"
	realRepo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(realRepo, "keep.txt"), []byte("real"), 0o644))

	var removed []string
	projects := &fakeDeleteProjects{projects: map[string]domain.Project{
		"p1": {ID: "p1", Name: "demo", Path: realRepo},
	}}
	repos := &fakeDeleteRepos{repos: []domain.Repository{
		{ID: "r1", ProjectID: "p1", Path: realRepo},
	}}
	workspaces := &fakeDeleteWorkspaces{workspaces: []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: realRepo},
	}}
	uc := project.NewDelete(project.DeleteDeps{
		Projects:    projects,
		Repos:       repos,
		Workspaces:  workspaces,
		Git:         &fakeDeleteGit{},
		CrowbarHome: func() (string, error) { return home, nil },
		RemoveAll: func(path string) error {
			removed = append(removed, path)
			return nil
		},
	})

	require.NoError(t, uc.Delete(context.Background(), "p1"))

	assert.Equal(t, []string{filepath.Join(home, "projects", "p1")}, removed,
		"only the crowbar project dir may be removed")
	for _, p := range removed {
		assert.NotEqual(t, realRepo, p, "the real repo path must never be removed")
	}
	// The real repo checkout must still exist on disk.
	_, statErr := os.Stat(filepath.Join(realRepo, "keep.txt"))
	require.NoError(t, statErr, "the real repo directory must survive")
}

func TestProjectDelete_WorkspaceRecordDeleteError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.delErr = errors.New("db down")
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-main", RepoID: "r1", ProjectID: "p1", Branch: "main", WorktreePath: deleteRepoPath, Status: domain.WorkspaceStatusLocked},
	}

	err := f.uc.Delete(context.Background(), "p1")
	require.Error(t, err)
	assert.Empty(t, f.repos.deleted)
	assert.Empty(t, f.projects.deleted)
}

// TestProjectDelete_FindProjectError_Aborts covers a lookup failure (e.g. a DB
// error) at the very top of Delete, distinct from the not-found case above.
func TestProjectDelete_FindProjectError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.projects.findErr = errors.New("db down")

	err := f.uc.Delete(context.Background(), "p1")

	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound, "a lookup failure is not a not-found")
}

// TestProjectDelete_ListReposError_Aborts covers projectRepos surfacing a
// repository listing failure before any record is touched.
func TestProjectDelete_ListReposError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.findErr = errors.New("db down")

	err := f.uc.Delete(context.Background(), "p1")

	require.Error(t, err)
	assert.Empty(t, f.workspaces.deleted)
	assert.Empty(t, f.projects.deleted)
}

// TestProjectDelete_ListWorkspacesError_Aborts covers deleteWorkspaces
// surfacing a workspace listing failure before any workspace row is removed.
func TestProjectDelete_ListWorkspacesError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.listErr = errors.New("db down")

	err := f.uc.Delete(context.Background(), "p1")

	require.Error(t, err)
	assert.Empty(t, f.repos.deleted)
	assert.Empty(t, f.projects.deleted)
}

// TestProjectDelete_RepoRecordDeleteError_AbortsBeforeProjectRow covers a repo
// row failing to delete: the project row itself must not be removed, so a
// retry can still find the project and its still-owned repo.
func TestProjectDelete_RepoRecordDeleteError_AbortsBeforeProjectRow(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.delErr = errors.New("db down")

	err := f.uc.Delete(context.Background(), "p1")

	require.Error(t, err)
	assert.Empty(t, f.projects.deleted, "the project row must survive a failed repo cascade")
}

// TestProjectDelete_ProjectRecordDeleteError_Surfaces covers the project row
// itself refusing to delete after every repo/workspace row is already gone.
func TestProjectDelete_ProjectRecordDeleteError_Surfaces(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.projects.delErr = errors.New("db down")

	err := f.uc.Delete(context.Background(), "p1")

	require.Error(t, err)
	assert.Equal(t, []string{"r1"}, f.repos.deleted, "the repo cascade must still have run")
}

// TestProjectDelete_ForceDeleteBranchFailure_StillDeletesRecords mirrors
// TestProjectDelete_WorktreeRemoveFailure_StillDeletesRecords for the second
// git step: a branch that refuses to force-delete must not block the record
// cascade, since disk teardown here is explicitly best-effort.
func TestProjectDelete_ForceDeleteBranchFailure_StillDeletesRecords(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.git.forceDeleteErr = errors.New("branch checked out elsewhere")
	crowbarPath := "/home/u/.crowbar/projects/github.com/test/repo/workspaces/w-child"
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-child", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", WorktreePath: crowbarPath},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Equal(t, []string{crowbarPath}, f.git.removedWorktrees,
		"the worktree remove must still have run")
	assert.Equal(t, []string{"w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// TestProjectDelete_OrphanedWorkspaceWithNoOwnedRepo_RecordOnly covers a
// workspace whose RepoID does not appear among the project's owned repos (a
// data inconsistency — e.g. its repo row was already removed independently).
// removeWorktreeIfCrowbarManaged must skip disk teardown for it rather than
// look up a repo path that doesn't exist, while the workspace record cascade
// still proceeds.
func TestProjectDelete_OrphanedWorkspaceWithNoOwnedRepo_RecordOnly(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.workspaces = []domain.Workspace{
		{
			ID: "w-orphan", RepoID: "does-not-exist", ProjectID: "p1",
			Branch: "feature/x", WorktreePath: "/home/u/.crowbar/projects/x/workspaces/w-orphan",
		},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.removedWorktrees, "there is no repo path to remove a worktree against")
	assert.Empty(t, f.git.deletedBranches)
	assert.Equal(t, []string{"w-orphan"}, f.workspaces.deleted,
		"the workspace record cascade must still proceed")
}

// TestProjectDelete_NoCrowbarHomeConfigured_SkipsAllDiskTeardown covers the
// nil-CrowbarHome case for both disk-teardown call sites (the per-workspace
// worktree removal and the final project directory removal): with no way to
// resolve crowbar home, disk teardown is skipped entirely, but the record
// cascade must still complete.
func TestProjectDelete_NoCrowbarHomeConfigured_SkipsAllDiskTeardown(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:   f.projects,
		Repos:      f.repos,
		Workspaces: f.workspaces,
		Git:        f.git,
		// CrowbarHome deliberately left nil.
	})
	f.workspaces.workspaces = []domain.Workspace{
		{
			ID: "w-child", RepoID: "r1", ProjectID: "p1",
			Branch: "feature/x", WorktreePath: "/home/u/.crowbar/projects/p1/workspaces/w-child",
		},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.removedWorktrees, "no crowbar home means no basis to identify a managed worktree")
	assert.Equal(t, []string{"w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// TestProjectDelete_CrowbarHomeError_SkipsDiskTeardown covers CrowbarHome
// itself erroring (as opposed to being unset) for both disk-teardown sites.
func TestProjectDelete_CrowbarHomeError_SkipsDiskTeardown(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return "", errors.New("home: boom") },
	})
	crowbarPath := "/home/u/.crowbar/projects/p1/workspaces/w-child"
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-child", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", WorktreePath: crowbarPath},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Empty(t, f.git.removedWorktrees, "a broken crowbar-home lookup must not be treated as managed")
	assert.Equal(t, []string{"w-child"}, f.workspaces.deleted)
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// TestProjectDelete_RemoveProjectDirFailure_IsLoggedNotFatal covers RemoveAll
// itself failing: the records are already gone by the time disk teardown
// runs, so a stale directory must not surface as a Delete error.
func TestProjectDelete_RemoveProjectDirFailure_IsLoggedNotFatal(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return "/home/u/.crowbar", nil },
		RemoveAll:   func(string) error { return errors.New("disk gremlin") },
	})

	err := f.uc.Delete(context.Background(), "p1")

	require.NoError(t, err, "a failed directory removal must not fail the whole delete")
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// TestRegression_ProjectDelete_RemoveProjectDir_RefusesPathTraversalEscape
// pins a safety guard: removeProjectDir refuses to run RemoveAll unless the
// resolved directory still lives under crowbarHome. A projectID carrying path
// traversal segments (however it got there — a corrupt row, a future caller
// that forgets to validate) must never let this rm -rf escape ~/.crowbar.
func TestRegression_ProjectDelete_RemoveProjectDir_RefusesPathTraversalEscape(t *testing.T) {
	home := "/home/u/.crowbar"
	var removed []string
	projects := &fakeDeleteProjects{projects: map[string]domain.Project{
		"../../etc": {ID: "../../etc", Name: "evil", Path: "/home/u/proj/repo"},
	}}
	uc := project.NewDelete(project.DeleteDeps{
		Projects:    projects,
		Repos:       &fakeDeleteRepos{},
		Workspaces:  &fakeDeleteWorkspaces{},
		Git:         &fakeDeleteGit{},
		CrowbarHome: func() (string, error) { return home, nil },
		RemoveAll: func(path string) error {
			removed = append(removed, path)
			return nil
		},
	})

	require.NoError(t, uc.Delete(context.Background(), "../../etc"))

	assert.Empty(t, removed, "a projectID that resolves outside crowbarHome must never reach RemoveAll")
}
