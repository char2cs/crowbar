package project_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestImportRepo_DuplicatePath_IsNoOp(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "p1", Name: "P", Path: "/root"}))
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "main", Head: "h1"}}
	prov.Protected = []string{"main"}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner: func(string) defaultbranch.RefRunner {
			return func(...string) (string, bool) { return "", false }
		},
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})

	r1, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "p1", Path: "/root/repoA"}, &recordedProgress{})
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	wsCount := len(ws.Created)

	r2, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "p1", Path: "/root/repoA"}, &recordedProgress{})
	require.NoError(t, err)
	assert.Equal(t, r1.ID, r2.ID, "re-add returns the already-imported repo")
	assert.Len(t, repos.Saved, 1, "no duplicate repository row")
	assert.Len(t, ws.Created, wsCount, "no duplicate workspaces")
}

func TestImportRepo_UnknownProject_Errors(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover: func(root string, maxDepth int) ([]string, error) {
			return nil, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "missing", Path: "/root/repoA"}, &recordedProgress{})
	require.Error(t, err)
	assert.Empty(t, repos.Saved)
}

func TestImportRepo_LoadProjectError_Errors(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	projects.FindErr = errors.New("db down")
	repos := mocks.NewRepositoryStore()
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover: func(root string, maxDepth int) ([]string, error) {
			return nil, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: "/root/repoA"}, &recordedProgress{})
	require.Error(t, err)
	assert.Empty(t, repos.Saved)
}

func TestCheckRepoImportable_NoRepos(t *testing.T) {
	_, _, _, _, _, uc := newImport(t)

	require.NoError(t, uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA"),
		"an empty store cannot conflict with anything")
}

func TestCheckRepoImportable_SameProjectIsNotAConflict(t *testing.T) {
	_, repos, _, _, _, uc := newImport(t)
	repos.Saved = []domain.Repository{{ID: "r1", ProjectID: "proj-1", Path: "/repoA"}}

	require.NoError(t, uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA"))
}

func TestCheckRepoImportable_DifferentPathIsNotAConflict(t *testing.T) {
	_, repos, _, _, _, uc := newImport(t)
	repos.Saved = []domain.Repository{{ID: "r1", ProjectID: "proj-2", Path: "/repoB"}}

	require.NoError(t, uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA"))
}

func TestCheckRepoImportable_OtherProjectRefusedAndNamed(t *testing.T) {
	projects, repos, _, _, _, uc := newImport(t)
	projects.Saved = []domain.Project{{ID: "proj-2", Name: "Other Project"}}
	repos.Saved = []domain.Repository{{ID: "r1", ProjectID: "proj-2", Path: "/repoA"}}

	err := uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA")

	require.ErrorIs(t, err, project.ErrRepoAlreadyImported)
	assert.Contains(t, err.Error(), "Other Project",
		"the refusal names the project holding the folder, so the user knows where it went")
}

func TestCheckRepoImportable_OtherProjectUnnamedStillRefused(t *testing.T) {
	_, repos, _, _, _, uc := newImport(t)
	repos.Saved = []domain.Repository{{ID: "r1", ProjectID: "proj-2", Path: "/repoA"}}

	err := uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA")

	require.ErrorIs(t, err, project.ErrRepoAlreadyImported)
	assert.NotContains(t, err.Error(), "in the project")
}

func TestCheckRepoImportable_ProjectLookupErrorStillRefuses(t *testing.T) {
	projects, repos, _, _, _, uc := newImport(t)
	projects.FindErr = errors.New("boom")
	repos.Saved = []domain.Repository{{ID: "r1", ProjectID: "proj-2", Path: "/repoA"}}

	err := uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA")

	require.ErrorIs(t, err, project.ErrRepoAlreadyImported)
	assert.NotContains(t, err.Error(), "in the project")
}

func TestCheckRepoImportable_ARepoReadErrorIsReturned(t *testing.T) {
	_, repos, _, _, _, uc := newImport(t)
	repos.FindErr = errors.New("boom")

	require.ErrorContains(t, uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA"), "boom")
}

func TestImportRepo_FolderAlreadyOwnedByAnotherProject_RefusesWithNoNewRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Name: "First"}))
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-2", Name: "Second"}))
	require.NoError(t, repos.Save(context.Background(), domain.Repository{ID: "r1", ProjectID: "proj-1", Path: "/shared/repo"}))

	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:       statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-2", Path: "/shared/repo"}, &recordedProgress{})

	require.ErrorIs(t, err, project.ErrRepoAlreadyImported)
	assert.Contains(t, err.Error(), "First", "the refusal names the project that already owns the folder")
	require.Len(t, repos.Saved, 1, "no second row for the same folder under a different project")
}

func TestImport_CrowbarHomeError_SkipsIconAndProtectedWorktrees(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov := mocks.NewProviderEngine()
	prov.Protected = []string{"main"}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  ws,
		Git:         git,
		Provider:    prov,
		CrowbarHome: func() (string, error) { return "", errors.New("home boom") },
		Discover:    func(string, int) ([]string, error) { return []string{"/repoA"}, nil },
		RefRunner:   noRefRunner,
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a broken crowbar-home lookup must degrade, not fail the import")
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon, "no icon can be written with no home to derive its path from")
	for _, w := range ws.Created {
		assert.NotEqual(t, "main", w.Branch, "no protected-branch worktree is provisioned with no crowbar home")
	}
}

func TestImport_CrowbarHomeIsAFile_IconAndSiblingScanBothFail(t *testing.T) {
	brokenHome := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(brokenHome, []byte("x"), 0o644))
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "favicon.svg"), []byte("<svg/>"), 0o644))

	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "feature", Head: "h1"}}
	prov := mocks.NewProviderEngine()
	prov.Protected = []string{"main"}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  ws,
		Git:         git,
		Provider:    prov,
		CrowbarHome: func() (string, error) { return brokenHome, nil },
		Discover:    func(string, int) ([]string, error) { return []string{repoDir}, nil },
		RefRunner:   noRefRunner,
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a broken (non-directory) crowbar home must degrade, not fail the import")
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon,
		"the icon dir cannot be created under a home that is itself a file")
	for _, w := range ws.Created {
		assert.NotEqual(t, "main", w.Branch,
			"the sibling scan under a broken home must fail the protected branch, not provision it")
	}
}

func TestImport_AvatarFetchError_LeavesGeneratedAvatar(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()

	repos := mocks.NewRepositoryStore()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return home, nil },
		FetchAvatarBytes: func(context.Context, string) ([]byte, string, error) {
			return nil, "", errors.New("network boom")
		},
		Discover:  func(string, int) ([]string, error) { return []string{repoDir}, nil },
		RefRunner: noRefRunner,
		Now:       func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:      statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "an avatar-fetch failure must not fail the import")
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon)
}

func TestImportRepo_AnExistingRepoLookupErrorFailsTheImport(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	repos.FindErr = errors.New("db down")
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "main", Head: "h1"}}

	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        git,
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:       statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: "/root/repoA"}, &recordedProgress{})
	require.ErrorContains(t, err, "db down")
	assert.Empty(t, repos.Saved)
}

func TestImport_NoWorktreeMatchesRepoPath_HomeHasEmptyBranch(t *testing.T) {
	_, ws, git, prov, uc := newImportForCoverage(t)
	git.Worktrees = nil
	prov.Protected = nil

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var home *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].IsDefault {
			home = &ws.Created[i]
		}
	}
	require.NotNil(t, home, "the repo home is still adopted even with no matching worktree entry")
	assert.Empty(t, home.Branch, "with no worktree entry to read a branch from, it is left empty")
}

func TestImportRepo_RecordsRealOriginRemoteURL(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	const remoteURL = "https://github.com/char2cs/example.git"
	runGit(t, repoDir, "remote", "add", "origin", remoteURL)

	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}

	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        git,
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:       statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: repoDir}, &recordedProgress{})
	require.NoError(t, err)
	assert.Equal(t, remoteURL, repo.RemoteURL, "the repo's real origin remote must be recorded")
	require.Len(t, repos.Saved, 1)
	assert.Equal(t, remoteURL, repos.Saved[0].RemoteURL)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

func TestImportRepo_IconWriteSkipped_WhenCrowbarHomeUnavailable(t *testing.T) {
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "favicon.svg"), []byte("<svg>icon</svg>"), 0o644))

	repos := mocks.NewRepositoryStore()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return "", errors.New("home boom") },
		Discover: func(root string, maxDepth int) ([]string, error) {
			return []string{repoDir}, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "an unresolvable crowbar home must not fail the import")
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon,
		"the icon cannot be written with nowhere under crowbar home to put it")
}

type workspaceOutcome struct {
	ID, Branch string
	Err        error
}

type recordedProgress struct {
	outcomes []workspaceOutcome
}

func (r *recordedProgress) Workspace(id, branch string, err error) {
	r.outcomes = append(r.outcomes, workspaceOutcome{ID: id, Branch: branch, Err: err})
}
