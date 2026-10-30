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
	// Re-adding a folder already imported under the project is a no-op: it returns
	// the existing repo and creates no duplicate row or duplicate workspaces.
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

// TestImportRepo_UnknownProject_Errors pins that adding a repo under an unknown
// project id returns an error and persists nothing.
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

// TestImportRepo_LoadProjectError_Errors pins that a project-store lookup error
// surfaces from ImportRepo and persists nothing.
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

	// Re-adding a folder the SAME project already has is a dedup (importOneRepo
	// returns the existing row), never a refusal.
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
	// The owning project row is absent, so its name cannot be resolved. The
	// refusal still stands — it just loses the pointer to where the folder lives.
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

// A read failure is an error, not "importable": guessing would let two
// projects share the folder.
func TestCheckRepoImportable_ARepoReadErrorIsReturned(t *testing.T) {
	_, repos, _, _, _, uc := newImport(t)
	repos.FindErr = errors.New("boom")

	require.ErrorContains(t, uc.CheckRepoImportable(context.Background(), "proj-1", "/repoA"), "boom")
}

// TestImportRepo_FolderAlreadyOwnedByAnotherProject_RefusesWithNoNewRow proves
// that importOneRepo's own CheckRepoImportable call (reached via ImportRepo,
// not called directly) refuses a folder another project has already imported,
// and persists nothing for the second project.
func TestImportRepo_FolderAlreadyOwnedByAnotherProject_RefusesWithNoNewRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Name: "First"}))
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-2", Name: "Second"}))
	// proj-1 already owns /shared/repo.
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

// TestImport_CrowbarHomeError_SkipsIconAndProtectedWorktrees proves that when
// CrowbarHome itself errors, BOTH the repo-icon write and the protected-branch
// worktree provisioning degrade cleanly: the repo is still imported (with a
// generated avatar and no managed worktrees), never failing the batch.
func TestImport_CrowbarHomeError_SkipsIconAndProtectedWorktrees(t *testing.T) {
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov := mocks.NewProviderEngine()
	prov.Protected = []string{"main"} // unheld -> would normally get a managed worktree
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

// TestImport_CrowbarHomeIsAFile_IconAndSiblingScanBothFail proves the DISTINCT
// degrade when CrowbarHome resolves to a real, non-empty path that is simply
// unusable as a directory (here: an existing plain file): the icon write's own
// os.MkdirAll fails, and the protected-branch worktree provisioning's sibling
// scan (os.ReadDir) fails the same way — both surfacing the real os error
// rather than the "home unset" degrade above, yet the import still succeeds.
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

// A failure to read the prior imports fails the import and saves nothing.
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
	git.Worktrees = nil // no entry at all — not even the repo's own path
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

// TestImportRepo_RecordsRealOriginRemoteURL proves gitRemoteURL's success
// return: with a REAL git repository configured with an origin remote at the
// import path, the persisted Repository carries that exact remote URL. Every
// other import test in this package points at a directory with no real .git
// at all, which only ever exercises gitRemoteURL's failure branch.
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

// TestImportRepo_IconWriteSkipped_WhenCrowbarHomeUnavailable proves writeRepoIcon
// degrades cleanly when CrowbarHome cannot be resolved: a local icon IS found on
// disk, but with nowhere under ~/.crowbar to write it the repo still imports,
// just with AvatarHasIcon left false (the generated label/color avatar).
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

// workspaceOutcome is one protected branch's outcome as ImportRepo reports it.
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
