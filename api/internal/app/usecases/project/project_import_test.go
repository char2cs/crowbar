package project_test

import (
	"context"
	"errors"
	"os"
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

func newImport(
	t *testing.T,
) (
	*mocks.ProjectStore,
	*mocks.RepositoryStore,
	*mocks.WorkspaceRepo,
	*mocks.GitEngine,
	*mocks.ProviderEngine,
	project.ImportUsecase,
) {
	t.Helper()
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()
	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover: func(
			root string,
			maxDepth int,
		) ([]string, error) {
			return []string{root + "/repoA"}, nil
		},
		RefRunner: func(
			repoPath string,
		) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) {
				if len(args) > 0 && args[0] == "symbolic-ref" && len(args) == 1 {
					return "refs/remotes/origin/main", true
				}
				return "", false
			}
		},
		Now: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
		Stat: statExists,
	})
	return projects, repos, ws, git, prov, uc
}

// statExists stubs ImportDeps.Stat so the fake "/root" import path passes the
// existence check without touching the real filesystem.
func statExists(
	_ string,
) (os.FileInfo, error) {
	return nil, nil
}

func TestImport_CreatesProjectReposAndAdoptsWorktrees(
	t *testing.T,
) {
	projects, repos, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/repoA/wt-feature", Branch: "feature", Head: "h2"},
	}
	git.MergeBaseSha = "forksha"
	prov.Protected = []string{"main"}

	project, err := uc.Import(context.Background(), "My Project", "/root")
	require.NoError(t, err)

	assert.Equal(t, "My Project", project.Name)
	assert.Equal(t, "/root", project.Path)
	assert.Len(t, projects.Saved, 1)

	require.Len(t, repos.Saved, 1)
	repo := repos.Saved[0]
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, project.ID, repo.ProjectID)
	assert.NotEmpty(t, repo.DefaultBranch)
	assert.Equal(t, "main", repo.DefaultBranch)
	assert.NotEmpty(t, repo.AvatarLabel)
	assert.NotEmpty(t, repo.AvatarColor)

	require.Len(t, ws.Created, 2)
	byBranch := map[string]bool{}
	for _, w := range ws.Created {
		byBranch[w.Branch] = w.Status == domain.WorkspaceStatusLocked
	}
	assert.True(t, byBranch["main"])
	assert.False(t, byBranch["feature"])
}

func TestImport_ProjectSaveError(
	t *testing.T,
) {
	projects, _, _, _, _, uc := newImport(t)
	projects.SaveErr = errors.New("boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}

func TestImport_DiscoverError(
	t *testing.T,
) {
	uc := project.NewImport(project.ImportDeps{
		Projects:   mocks.NewProjectStore(),
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover: func(
			root string,
			maxDepth int,
		) ([]string, error) {
			return nil, errors.New("walk failed")
		},
		RefRunner: func(
			repoPath string,
		) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  time.Now,
		Stat: statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}

func TestImport_FolderNotExist_PersistsNothing(
	t *testing.T,
) {
	// BUG-006/BUG-005: importing a nonexistent path must fail with the clean
	// ErrFolderNotFound sentinel BEFORE the project row is persisted, so a
	// failed import leaves no project behind.
	projects, repos, ws, _, _, _ := newImport(t)
	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover: func(
			root string,
			maxDepth int,
		) ([]string, error) {
			return nil, nil
		},
		RefRunner: func(
			repoPath string,
		) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now: time.Now,
	})

	_, err := uc.Import(context.Background(), "P", "/nonexistent/path/x")
	require.ErrorIs(t, err, project.ErrFolderNotFound)
	assert.Equal(t, "folder does not exist", err.Error())
	assert.Empty(t, projects.Saved, "failed import must not persist the project")
	assert.Empty(t, repos.Saved)
	assert.Empty(t, ws.Created)
}

func TestImport_RepoSaveError_IsTolerated(
	t *testing.T,
) {
	// A repo-save failure is inside importOneRepo — best-effort: the project
	// is still returned with no error and zero repos persisted.
	projects, repos, _, _, _, uc := newImport(t)
	repos.SaveErr = errors.New("repo boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Empty(t, repos.Saved)
}

func TestImport_WorktreeListError_IsTolerated(
	t *testing.T,
) {
	// A worktree-list failure is inside importOneRepo — best-effort: the
	// project and the repo row are returned; no workspaces are adopted.
	projects, repos, ws, git, _, uc := newImport(t)
	git.WorktreeListErr = errors.New("wt boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Len(t, repos.Saved, 1)
	assert.Empty(t, ws.Created)
}

func TestImport_ProtectedBranchesError_IsTolerated(
	t *testing.T,
) {
	// A protected-branches failure is inside importOneRepo — best-effort:
	// the project and repo row are returned; no workspaces are adopted.
	projects, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov.ProtectedErr = errors.New("prov boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Len(t, repos.Saved, 1)
	assert.Empty(t, ws.Created)
}

func TestImport_MergeBaseErrorIsTolerated(
	t *testing.T,
) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h2"}}
	git.MergeBaseErr = errors.New("no merge base")
	prov.Protected = nil

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 1)
	assert.Empty(t, ws.Created[0].ForkPointSha)
}

func TestImport_DetachedWorktreeSkipped(
	t *testing.T,
) {
	_, _, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "", Head: "detached"},
		{Path: "/repoA/wt", Branch: "feature", Head: "h2"},
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 1)
	assert.Equal(t, "feature", ws.Created[0].Branch)
}

func TestImport_PrunableWorktreeSkipped(
	t *testing.T,
) {
	// A prunable worktree points at a checkout that no longer exists on disk
	// (e.g. a deleted temp dir from a test run). Adopting it would create a
	// workspace whose every file/git read fails, so it must be skipped.
	_, _, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/gone/wt", Branch: "dead", Head: "h2", Prunable: true},
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 1)
	assert.Equal(t, "main", ws.Created[0].Branch)
}

func TestImport_WorkspaceCreateError_IsTolerated(
	t *testing.T,
) {
	// A workspace-create failure is inside importOneRepo — best-effort: the
	// project and repo row are returned; no workspaces are adopted.
	projects, repos, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	ws.CreateErr = errors.New("ws boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Len(t, repos.Saved, 1)
	assert.Empty(t, ws.Created)
}

func TestImport_WritesRepoIconToEntityDir(t *testing.T) {
	// A local repo icon (favicon.svg) on disk must be copied into the
	// entity-scoped icon path <home>/projects/<P>/<R>/icon, and the repo row
	// must record AvatarHasIcon=true.
	home := t.TempDir()
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "favicon.svg"), []byte("<svg>icon</svg>"), 0o644))

	repos := mocks.NewRepositoryStore()
	uc := project.NewImport(project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return home, nil },
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
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)

	saved := repos.Saved[0]
	assert.True(t, saved.AvatarHasIcon)

	iconPath := filepath.Join(home, "projects", saved.ProjectID, saved.ID, "icon")
	data, readErr := os.ReadFile(iconPath)
	require.NoError(t, readErr)
	assert.Equal(t, "<svg>icon</svg>", string(data))
}

func TestImport_DefaultsToGithubAvatar(t *testing.T) {
	// No local icon: import must best-effort fetch the GitHub owner avatar
	// bytes and write them to the entity icon path, setting AvatarHasIcon=true.
	home := t.TempDir()
	repoDir := t.TempDir() // no icon files inside

	repos := mocks.NewRepositoryStore()
	fetched := false
	uc := project.NewImport(project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return home, nil },
		FetchAvatarBytes: func(_ context.Context, _ string) ([]byte, string, error) {
			fetched = true
			return []byte("PNGDATA"), "image/png", nil
		},
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
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	assert.True(t, fetched, "the github avatar fetcher must be invoked when no local icon exists")

	saved := repos.Saved[0]
	assert.True(t, saved.AvatarHasIcon)
	iconPath := filepath.Join(home, "projects", saved.ProjectID, saved.ID, "icon")
	data, readErr := os.ReadFile(iconPath)
	require.NoError(t, readErr)
	assert.Equal(t, "PNGDATA", string(data))
}

func TestImport_AvatarFetchFailureLeavesGeneratedAvatar(t *testing.T) {
	// No local icon AND the github fetch yields nothing: the repo must still
	// be saved with AvatarHasIcon=false (generated label/color avatar) and the
	// import must NOT fail.
	home := t.TempDir()
	repoDir := t.TempDir() // no icon files

	repos := mocks.NewRepositoryStore()
	uc := project.NewImport(project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return home, nil },
		FetchAvatarBytes: func(_ context.Context, _ string) ([]byte, string, error) {
			return nil, "", nil // degraded: no gh/origin
		},
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
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)

	saved := repos.Saved[0]
	assert.False(t, saved.AvatarHasIcon)
	assert.NotEmpty(t, saved.AvatarLabel)
	assert.NotEmpty(t, saved.AvatarColor)

	iconPath := filepath.Join(home, "projects", saved.ProjectID, saved.ID, "icon")
	_, statErr := os.Stat(iconPath)
	assert.True(t, os.IsNotExist(statErr), "no icon file should be written on fetch failure")
}

func TestImport_AutoImportsProtectedBranchStubs(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// Only "main" is a local worktree; "develop" is protected but not local
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	// Should have created 2 workspaces: main (adopted) + develop (stub)
	require.Len(t, ws.Created, 2)
	byBranch := map[string]bool{}
	for _, w := range ws.Created {
		byBranch[w.Branch] = w.Status == domain.WorkspaceStatusLocked
	}
	assert.True(t, byBranch["main"])
	assert.True(t, byBranch["develop"])
}

func TestImport_SkipsStubWhenAlreadyAdopted(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// "develop" is both local and protected — should not be created twice
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Len(t, ws.Created, 1)
}

func TestImport_AvatarFallsBackToGeneratedWhenNoFetcher(t *testing.T) {
	// The default newImport fixture has no CrowbarHome/FetchAvatarBytes wired,
	// and the fake repoA path has no local icon, so the repo falls back to a
	// generated avatar (AvatarHasIcon=false) without error.
	_, repos, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	assert.False(t, repos.Saved[0].AvatarHasIcon)
}

// TestImportRepo_AdoptsDefaultBranchWorkspace pins the OOBE add-repo path: a
// repo whose checked-out branch is "develop" is imported under an existing
// project — ImportRepo persists the repo row and adopts the branch as a
// workspace (§14 Step 3). It loads the project by id from the project store.
func TestImportRepo_AdoptsDefaultBranchWorkspace(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	proj := domain.Project{ID: "proj-1", Name: "P", Path: "/root"}
	require.NoError(t, projects.Save(context.Background(), proj))

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/root/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover: func(root string, maxDepth int) ([]string, error) {
			return nil, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.NoError(t, err)

	require.Len(t, repos.Saved, 1)
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, "proj-1", repo.ProjectID)
	assert.Equal(t, repos.Saved[0].ID, repo.ID)

	require.Len(t, ws.Created, 1)
	assert.Equal(t, "develop", ws.Created[0].Branch)
	assert.Equal(t, repo.ID, ws.Created[0].RepoID)
}

// TestImportRepo_SetsGithubAvatarBestEffort pins that adding a repo with no
// local icon best-effort fetches the GitHub owner avatar bytes and writes them
// to the entity icon path, setting AvatarHasIcon=true.
func TestImportRepo_SetsGithubAvatarBestEffort(
	t *testing.T,
) {
	home := t.TempDir()
	repoDir := t.TempDir() // no local icon files inside

	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))

	fetched := false
	uc := project.NewImport(project.ImportDeps{
		Projects:    projects,
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		CrowbarHome: func() (string, error) { return home, nil },
		FetchAvatarBytes: func(_ context.Context, _ string) ([]byte, string, error) {
			fetched = true
			return []byte("PNGDATA"), "image/png", nil
		},
		Discover: func(root string, maxDepth int) ([]string, error) {
			return nil, nil
		},
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), "proj-1", repoDir)
	require.NoError(t, err)
	assert.True(t, fetched, "the github avatar fetcher must run when no local icon exists")
	assert.True(t, repo.AvatarHasIcon)

	iconPath := filepath.Join(home, "projects", "proj-1", repo.ID, "icon")
	data, readErr := os.ReadFile(iconPath)
	require.NoError(t, readErr)
	assert.Equal(t, "PNGDATA", string(data))
}

// TestImportRepo_UnknownProject_Errors pins that adding a repo under an unknown
// project id returns an error and persists nothing.
func TestImportRepo_UnknownProject_Errors(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	uc := project.NewImport(project.ImportDeps{
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

	_, err := uc.ImportRepo(context.Background(), "missing", "/root/repoA")
	require.Error(t, err)
	assert.Empty(t, repos.Saved)
}

func TestImportRepo_FlagsMainWorktreeAsDefault(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"}))

	// The main worktree is the entry whose Path == repo.Path (/root/repoA).
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/root/repoA", Branch: "develop", Head: "h1"},
		{Path: "/root/wt-x", Branch: "feature/x", Head: "h2"},
	}
	prov.Protected = []string{"staging"} // a protected branch with no local worktree

	uc := project.NewImport(project.ImportDeps{
		Projects: projects, Repos: repos, Workspaces: ws, Git: git, Provider: prov,
		Discover:  func(string, int) ([]string, error) { return nil, nil },
		RefRunner: func(string) defaultbranch.RefRunner { return func(...string) (string, bool) { return "", false } },
		Now:       func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:      statExists,
	})

	_, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.NoError(t, err)

	byBranch := map[string]bool{}
	for _, c := range ws.Created {
		byBranch[c.Branch] = c.IsDefault
	}
	assert.True(t, byBranch["develop"], "main-worktree workspace must be IsDefault")
	assert.False(t, byBranch["feature/x"], "non-main worktree must not be IsDefault")
	assert.False(t, byBranch["staging"], "protected stub must not be IsDefault")
}

// TestImportRepo_LoadProjectError_Errors pins that a project-store lookup error
// surfaces from ImportRepo and persists nothing.
func TestImportRepo_LoadProjectError_Errors(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	projects.FindErr = errors.New("db down")
	repos := mocks.NewRepositoryStore()
	uc := project.NewImport(project.ImportDeps{
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

	_, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.Error(t, err)
	assert.Empty(t, repos.Saved)
}

// TestImport_PartialRepoFailure verifies the best-effort guarantee (00 §5.1):
// when two repos are discovered and the first fails (git engine error), Import
// still returns the project with no error and the second repo IS fully adopted.
func TestImport_PartialRepoFailure(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	// repoA fails worktree listing; repoB succeeds with one worktree.
	git.WorktreeListFn = func(repoPath string) ([]gitengine.WorktreeEntry, error) {
		if repoPath == "/root/repoA" {
			return nil, errors.New("git failure on repoA")
		}
		return []gitengine.WorktreeEntry{
			{Path: repoPath, Branch: "main", Head: "h1"},
		}, nil
	}

	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover: func(
			root string,
			maxDepth int,
		) ([]string, error) {
			return []string{root + "/repoA", root + "/repoB"}, nil
		},
		RefRunner: func(
			repoPath string,
		) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	project, err := uc.Import(context.Background(), "My Project", "/root")

	// The import must succeed even though repoA failed.
	require.NoError(t, err)
	assert.Equal(t, "My Project", project.Name)
	assert.Len(t, projects.Saved, 1)

	// Both repo rows are saved (both get past Repos.Save before WorktreeList).
	require.Len(t, repos.Saved, 2, "both repo rows must be saved")
	repoNames := make([]string, 0, len(repos.Saved))
	for _, r := range repos.Saved {
		repoNames = append(repoNames, r.Name)
	}
	assert.Contains(t, repoNames, "repoB")

	// repoB's workspace (branch "main") must be adopted; repoA has none.
	require.Len(t, ws.Created, 1, "repoB workspace must be adopted")
	assert.Equal(t, "main", ws.Created[0].Branch)
}
