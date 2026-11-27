package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
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
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover: func(
			_ string,
			_ int,
		) ([]string, error) {
			return []string{"/repoA"}, nil
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
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})
	return projects, repos, ws, git, prov, uc
}

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
	git.RemoteBranches = map[string]bool{"develop": true}
	prov.Protected = []string{"main", "develop"}

	project, err := uc.Import(context.Background(), "My Project", "/root")
	require.NoError(t, err)

	assert.Equal(t, "My Project", project.Name)
	assert.Equal(t, "/root", project.Path)
	assert.Len(t, projects.Saved, 1)

	require.Len(t, repos.Saved, 1)
	repo := repos.Saved[0]
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, project.ID, repo.ProjectID)
	assert.Equal(t, "main", repo.DefaultBranch)

	require.Len(t, ws.Created, 4)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.Equal(t, "/root", ws.Created[0].WorktreePath)

	home := ws.Created[1]
	assert.True(t, home.IsDefault, "the repo home is the default workspace")
	assert.Equal(t, "/repoA", home.WorktreePath, "the repo home stays the repo folder")
	assert.Equal(t, "main", home.Branch, "the repo home stays on its branch (NOT detached)")
	assert.NotEqual(t, domain.WorkspaceStatusLocked, home.Status)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	var placeholder, managed domain.Workspace
	for _, w := range ws.Created {
		if w.Branch == "main" && !w.IsDefault {
			placeholder = w
		}
		if w.Branch == "develop" {
			managed = w
		}
	}
	assert.Equal(t, domain.WorkspaceStatusLocked, placeholder.Status)
	assert.Empty(t, placeholder.WorktreePath, "the home-held branch is a placeholder")
	assert.Equal(t, "/repoA", placeholder.HeldByPath)

	assert.Equal(t, domain.WorkspaceStatusLocked, managed.Status)
	assert.NotEqual(t, "/repoA", managed.WorktreePath)
	assert.Equal(t,
		filepath.Join("/crowbar-home", "projects", project.ID, "repoA", "develop", "worktree"),
		managed.WorktreePath)
}

func TestImport_SkipsNonProtectedLocalWorktrees(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
		{Path: "/repoA/wt1", Branch: "feature/x", Head: "h2"},
		{Path: "/repoA/wt2", Branch: "spike/y", Head: "h3"},
		{Path: "/repoA/wt3", Branch: "worktree-agent-abc", Head: "h4"},
	}
	prov.Protected = []string{"develop", "main"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	byBranch := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			byBranch[w.Branch] = w
		}
	}
	require.Contains(t, byBranch, "develop")
	require.Contains(t, byBranch, "main")
	assert.NotContains(t, byBranch, "feature/x", "non-protected local worktree is NOT imported")
	assert.NotContains(t, byBranch, "spike/y")
	assert.NotContains(t, byBranch, "worktree-agent-abc")

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["develop"].Status)
	assert.Empty(t, byBranch["develop"].WorktreePath, "the home-held develop is a placeholder")
	assert.Equal(t, "/repoA", byBranch["develop"].HeldByPath)

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["main"].Status)
	assert.NotEqual(t, "/repoA", byBranch["main"].WorktreePath, "unheld main gets a managed worktree")

	assert.Len(t, ws.Created, 4, "project home + repo home + develop placeholder + main managed")
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
	uc := newImportUsecase(t, project.ImportDeps{
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
	projects, repos, ws, _, _, _ := newImport(t)
	uc := newImportUsecase(t, project.ImportDeps{
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
	projects, repos, ws, git, _, uc := newImport(t)
	git.WorktreeListErr = errors.New("wt boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Empty(t, repos.Saved,
		"a repo whose worktree adoption fails must be rolled back, not left orphaned")
	require.Len(t, ws.Created, 1)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind,
		"only the home workspace is created when worktree adoption fails")
}

func TestImport_ProtectedBranchesError_ImportsRepoHomeOnly(
	t *testing.T,
) {
	projects, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov.ProtectedErr = errors.New("prov boom")

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	require.Len(t, repos.Saved, 1, "provider failure is soft: the repo home is still imported")
	require.Len(t, ws.Created, 2, "project home + repo home (no protected worktrees)")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "main", home.Branch, "home keeps its branch when protected info is unavailable")
	assert.Empty(t, git.Detached, "no blind detach when protected branches are unknown")
}

func TestImport_HomeOnNonProtectedBranch_NotDetached(
	t *testing.T,
) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h2"}}
	prov.Protected = nil

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3, "project home + repo home + default-branch managed worktree")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "feature", home.Branch, "a non-default home keeps its branch")
	assert.Empty(t, home.ForkPointSha)
	assert.Empty(t, git.Detached, "no detach for a non-default home branch")

	main := ws.Created[2]
	assert.Equal(t, "main", main.Branch, "the resolved default branch is provisioned even with no protected branches")
	assert.Equal(t, domain.WorkspaceStatusLocked, main.Status, "the default branch is locked")
	assert.NotEqual(t, "/repoA", main.WorktreePath, "the unheld default branch gets a managed worktree")
}

func TestImport_DetachedWorktreeSkipped(
	t *testing.T,
) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/repoA/wt", Branch: "", Head: "detached"},
	}
	prov.Protected = []string{"main"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3, "project home + repo home + main managed worktree")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.True(t, ws.Created[1].IsDefault, "repo home")
	assert.Equal(t, "main", ws.Created[2].Branch)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Created[2].Status)
	for _, w := range ws.Created {
		assert.NotEqual(t, "/repoA/wt", w.WorktreePath, "the unrelated non-main worktree is not imported")
	}
}

func TestImport_PrunableWorktreeSkipped(
	t *testing.T,
) {
	_, _, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/gone/wt", Branch: "dead", Head: "h2", Prunable: true},
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.Equal(t, "main", ws.Created[1].Branch)
	for _, w := range ws.Created {
		assert.NotEqual(t, "dead", w.Branch, "the prunable worktree's branch is never adopted")
		assert.NotEqual(t, "/gone/wt", w.WorktreePath, "the prunable worktree is never adopted")
	}
}

func TestImport_WorkspaceCreateError_IsTolerated(
	t *testing.T,
) {
	projects, repos, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.RepoID != "" {
			return domain.Workspace{}, errors.New("ws boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, ProjectID: in.ProjectID}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	project, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Equal(t, "P", project.Name)
	assert.Len(t, projects.Saved, 1)
	assert.Empty(t, repos.Saved,
		"a repo whose worktree adoption fails must be rolled back, not left orphaned")
	require.Len(t, ws.Created, 1)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind,
		"only the home workspace is created when repo-workspace creation fails")
}

func TestImport_WritesRepoIconToEntityDir(t *testing.T) {
	home := t.TempDir()
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
	home := t.TempDir()
	repoDir := t.TempDir()

	repos := mocks.NewRepositoryStore()
	fetched := false
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
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
		FetchAvatarBytes: func(_ context.Context, _ string) ([]byte, string, error) {
			return nil, "", nil
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

func TestImport_ProvisionsManagedWorktreesForProtectedBranches(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	require.Len(t, ws.Created, 4, "project home + repo home + main placeholder + develop managed worktree")
	assert.Empty(t, git.Detached, "the repo home is adopted in place, never force-detached")

	byBranch := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			byBranch[w.Branch] = w
		}
	}
	require.Contains(t, byBranch, "main")
	require.Contains(t, byBranch, "develop")

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["main"].Status, "main is a locked placeholder")
	assert.Empty(t, byBranch["main"].WorktreePath, "the home-held main is a placeholder with no worktree")
	assert.Equal(t, "/repoA", byBranch["main"].HeldByPath, "the placeholder records the home as holder")

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["develop"].Status, "develop is locked")
	assert.NotEqual(t, "/repoA", byBranch["develop"].WorktreePath, "develop gets a managed worktree, not the repo folder")
	assert.Contains(t, byBranch["develop"].WorktreePath, "/repoA/develop")
	assert.True(t, strings.HasSuffix(byBranch["develop"].WorktreePath, "/worktree"),
		"managed worktree path must end in the workspace-root's worktree leaf")
	assert.Empty(t, byBranch["develop"].HeldByPath, "a managed worktree has no holder")
}

func TestImport_DefaultProtectedBranch_HomeInPlaceAndPlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3, "project home + repo home (in place) + develop placeholder")

	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "develop", home.Branch, "the repo home keeps develop (adopted in place, not detached)")
	assert.Equal(t, "/repoA", home.WorktreePath)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	develop := ws.Created[2]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.Empty(t, develop.WorktreePath, "the home-held develop is a placeholder, not a managed worktree")
	assert.Equal(t, "/repoA", develop.HeldByPath)

	count := 0
	for _, w := range ws.Created {
		if w.Branch == "develop" && !w.IsDefault {
			count++
		}
	}
	assert.Equal(t, 1, count, "develop appears exactly once as a placeholder (never duplicated)")
}

func TestImport_NoProtectedBranches_DefaultBranchLockedManagedWorktree(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var mainWs *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "main" && !ws.Created[i].IsDefault {
			mainWs = &ws.Created[i]
		}
	}
	require.NotNil(t, mainWs, "the default branch must be provisioned even when the provider reports no protected branches")
	assert.Equal(t, domain.WorkspaceStatusLocked, mainWs.Status, "the default branch is locked/protected")
	assert.NotEqual(t, "/repoA", mainWs.WorktreePath, "the default branch gets a managed worktree, not the repo folder")
}

func TestImport_RemoteProtectedBranch_CheckedOutAtOriginRefAndForkPointRecorded(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"release"}
	git.RemoteTrackingBranches = map[string]bool{"release": true}
	git.RevParseShas = map[string]string{"origin/release": "deadbeef"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	assert.Contains(t, git.FetchedRefs, "release",
		"a remote protected branch's origin ref is refreshed before checkout")
	assert.NotContains(t, git.FastForwardedBranches, "release",
		"the local branch ref must NOT be moved by a fetch git refuses when it is checked out")
	require.Len(t, git.WorktreeAddAtRefs, 1,
		"a branch on origin must be checked out AT origin's ref, not at the local ref")
	assert.Equal(t, "release", git.WorktreeAddAtRefs[0].Branch)
	assert.Equal(t, "origin/release", git.WorktreeAddAtRefs[0].StartRef)

	var release *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			release = &ws.Created[i]
		}
	}
	require.NotNil(t, release)
	assert.Equal(t, "deadbeef", release.ForkPointSha,
		"the fork point is the commit the checkout actually landed on")
	assert.Contains(t, git.UpstreamsSet, "release",
		"`git worktree add -B <sha>` sets no tracking info, so the upstream must be set explicitly "+
			"or `git pull` in the locked worktree has nothing to merge with")
}

func TestImport_LocalOnlyProtectedBranch_ChecksOutLocalRef(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"release"}
	git.RevParseShas = map[string]string{"refs/heads/release": "localsha"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	assert.Empty(t, git.WorktreeAddAtRefs, "no origin ref to start at")
	require.Len(t, git.WorktreeAdds, 1)
	assert.Equal(t, "release", git.WorktreeAdds[0].Branch)

	var release *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			release = &ws.Created[i]
		}
	}
	require.NotNil(t, release)
	assert.Equal(t, "localsha", release.ForkPointSha)
}

func TestImport_LocalOnlyProtectedBranch_NotFetched(t *testing.T) {
	_, _, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"master"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.NotContains(t, git.FastForwardedBranches, "master", "a local-only protected branch is not fast-forwarded")
	require.Len(t, git.WorktreeAdds, 1)
	assert.Equal(t, "master", git.WorktreeAdds[0].Branch)
}

func TestImport_ProtectedWorktreeFailure_IsBestEffort(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main", "develop"}
	git.WorktreeAddErrByBranch = map[string]error{"main": errors.New("add boom")}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1, "the repo is kept despite a protected-worktree failure")
	managed := map[string]bool{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			managed[w.Branch] = true
		}
	}
	assert.False(t, managed["main"], "the failed branch is skipped")
	assert.True(t, managed["develop"], "other protected branches still get managed worktrees")
}

func TestImport_ProtectedRowFailure_CleansUpOrphanedWorktree(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.Protected {
			return domain.Workspace{}, errors.New("row boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault, WorktreePath: in.WorktreePath, Provisioning: domain.WorkspaceProvisioned}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, git.WorktreeAdds, 1, "the managed worktree was created on disk")
	require.Len(t, git.WorktreeRemoves, 1, "the orphaned worktree is cleaned up after the row failed")
	assert.Equal(t, git.WorktreeAdds[0].Path, git.WorktreeRemoves[0], "cleanup removes the same worktree path")
}

func TestImport_DetachFailure_DegradesAndStillImports(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov.Protected = []string{"main"}
	git.DetachErr = errors.New("fatal: cannot switch branch while merging")
	git.WorktreeAddErrByBranch = map[string]error{"main": errors.New("already used by worktree")}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a detach failure must not fail the import")
	require.Len(t, repos.Saved, 1, "the repo is still imported after a detach failure")
	require.GreaterOrEqual(t, len(ws.Created), 2)
	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "main", home.Branch, "the home keeps its branch when detach fails (degraded)")
	assert.NotEqual(t, domain.WorkspaceStatusLocked, home.Status, "the home is never locked")
	assert.Empty(t, git.Detached, "the detach did not succeed")
}

func TestImport_HomeRowFailure_NoDetachNoReattach(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "develop", Head: "h1"}}
	prov.Protected = []string{"develop"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.IsDefault {
			return domain.Workspace{}, errors.New("home row boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Empty(t, git.Detached, "the home is adopted in place, so nothing is ever detached")
	assert.Empty(t, git.CheckedOut, "no reattach: there was no detach to undo")
}

func TestImport_AvatarFallsBackToGeneratedWhenNoFetcher(t *testing.T) {
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

	uc := newImportUsecase(t, project.ImportDeps{
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
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: "/root/repoA"}, &recordedProgress{})
	require.NoError(t, err)

	require.Len(t, repos.Saved, 1)
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, "proj-1", repo.ProjectID)
	assert.Equal(t, repos.Saved[0].ID, repo.ID)

	require.Len(t, ws.Created, 2, "repo home (in place) + develop placeholder")
	home := ws.Created[0]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "develop", home.Branch, "the repo home keeps develop (adopted in place, not detached)")
	assert.Equal(t, repo.ID, home.RepoID)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	develop := ws.Created[1]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, repo.ID, develop.RepoID)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.Empty(t, develop.WorktreePath, "the home-held develop is a placeholder")
	assert.Equal(t, "/root/repoA", develop.HeldByPath, "the placeholder records the home as holder")
}

func TestImportRepo_AdoptionFailure_RollsBackRepo(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	nodes := mocks.NewNodePlacements()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(
		context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"},
	))
	git.WorktreeListErr = errors.New("wt boom")

	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Nodes:      nodes,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover:   func(root string, maxDepth int) ([]string, error) { return nil, nil },
		RefRunner: func(repoPath string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) { return "", false }
		},
		Now:  func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat: statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: "/root/repoA"}, &recordedProgress{})
	require.Error(t, err, "ImportRepo must propagate the adoption failure")
	assert.Empty(t, repos.Saved,
		"the repo row must be rolled back so retries cannot accumulate orphaned repos")
	assert.Empty(t, ws.Created)
	assert.Empty(t, nodes.Rows,
		"the repo's own Node row must be rolled back too, or it's a permanently orphaned position row")
}

func TestImportRepo_SetsGithubAvatarBestEffort(
	t *testing.T,
) {
	home := t.TempDir()
	repoDir := t.TempDir()

	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))

	fetched := false
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:    projects,
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
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

	repo, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: repoDir}, &recordedProgress{})
	require.NoError(t, err)
	assert.True(t, fetched, "the github avatar fetcher must run when no local icon exists")
	assert.True(t, repo.AvatarHasIcon)

	iconPath := filepath.Join(home, "projects", "proj-1", repo.ID, "icon")
	data, readErr := os.ReadFile(iconPath)
	require.NoError(t, readErr)
	assert.Equal(t, "PNGDATA", string(data))
}

func TestImportRepo_FlagsMainWorktreeAsDefault(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"}))

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/root/repoA", Branch: "develop", Head: "h1"},
		{Path: "/root/wt-x", Branch: "feature/x", Head: "h2"},
	}
	prov.Protected = []string{"staging"}

	uc := newImportUsecase(t, project.ImportDeps{
		Projects: projects, Repos: repos, Workspaces: ws, Git: git, Provider: prov,
		Discover:  func(string, int) ([]string, error) { return nil, nil },
		RefRunner: func(string) defaultbranch.RefRunner { return func(...string) (string, bool) { return "", false } },
		Now:       func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:      statExists,
	})

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: "/root/repoA"}, &recordedProgress{})
	require.NoError(t, err)

	byBranch := map[string]bool{}
	for _, c := range ws.Created {
		byBranch[c.Branch] = c.IsDefault
	}
	assert.True(t, byBranch["develop"], "main-worktree workspace must be IsDefault")
	assert.False(t, byBranch["feature/x"], "non-main worktree must not be IsDefault")
	assert.False(t, byBranch["staging"], "protected stub must not be IsDefault")
}

func TestImport_PartialRepoFailure(
	t *testing.T,
) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	git.WorktreeListFn = func(repoPath string) ([]gitengine.WorktreeEntry, error) {
		if repoPath == "/root/repoA" {
			return nil, errors.New("git failure on repoA")
		}
		return []gitengine.WorktreeEntry{
			{Path: repoPath, Branch: "main", Head: "h1"},
		}, nil
	}

	uc := newImportUsecase(t, project.ImportDeps{
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

	require.NoError(t, err)
	assert.Equal(t, "My Project", project.Name)
	assert.Len(t, projects.Saved, 1)

	require.Len(t, repos.Saved, 1, "the failed repo must be rolled back; only repoB persists")
	assert.Equal(t, "repoB", repos.Saved[0].Name)

	require.Len(t, ws.Created, 3, "project home + repoB home + repoB default-branch placeholder")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.Equal(t, "main", ws.Created[1].Branch)
}

func TestCreate_ProvisionesHomeWorkspace(t *testing.T) {
	_, _, ws, _, _, uc := newImport(t)

	dir := t.TempDir()
	_, err := uc.Create(context.Background(), "", "myproject", dir)
	require.NoError(t, err)

	require.Len(t, ws.Created, 1)
	require.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	require.Equal(t, dir, ws.Created[0].WorktreePath)
}

func TestImport_ProvisionesHomeWorkspace(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main"}

	_, err := uc.Import(context.Background(), "myproject", "/root")
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(ws.Created), 1)
	homeWS := ws.Created[0]
	require.Equal(t, domain.WorkspaceKindHome, homeWS.Kind)
	require.Equal(t, "/root", homeWS.WorktreePath)
	require.Empty(t, homeWS.RepoID, "home workspace must not be repo-scoped")
}

func TestImport_HomeHeldProtectedBranch_YieldsSinglePlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "develop", Head: "h1"}}
	prov.Protected = []string{"develop", "master"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	assert.Empty(t, git.Detached, "the repo home is NOT detached; it is adopted in place")

	var developRows, placeholders, managed []domain.Workspace
	for _, w := range ws.Created {
		if w.Branch == "develop" && !w.IsDefault {
			developRows = append(developRows, w)
		}
		if w.Status == domain.WorkspaceStatusLocked && w.WorktreePath == "" {
			placeholders = append(placeholders, w)
		}
		if w.Status == domain.WorkspaceStatusLocked && w.WorktreePath != "" {
			managed = append(managed, w)
		}
	}
	require.Len(t, developRows, 1, "the home-held protected branch yields exactly ONE develop row")
	require.Len(t, placeholders, 1, "develop is a single placeholder")
	assert.Equal(t, "develop", placeholders[0].Branch)
	assert.Equal(t, "/repoA", placeholders[0].HeldByPath, "placeholder records the home as holder")
	require.Len(t, managed, 1, "master (unheld) gets a managed worktree")
	assert.Equal(t, "master", managed[0].Branch)
}

func TestImport_ExternalHolder_YieldsPlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/some/external/wt", Branch: "release", Head: "h2"},
	}
	prov.Protected = []string{"release"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var placeholder *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			placeholder = &ws.Created[i]
		}
	}
	require.NotNil(t, placeholder)
	assert.Equal(t, domain.WorkspaceStatusLocked, placeholder.Status)
	assert.Empty(t, placeholder.WorktreePath)
	assert.Equal(t, "/some/external/wt", placeholder.HeldByPath)
}

func TestImport_DeadRegistrationPruned_ProvisionsCleanly(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	git.DeadRegistrations = map[string]string{"/deleted/wt-develop": "develop"}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop is provisioned, not dropped")
	assert.NotEmpty(t, managed.WorktreePath, "develop got a managed worktree after prune freed it")
	assert.Contains(t, git.Pruned, "/repoA", "prune ran before provisioning")
}

func TestImport_ParentFetchFails_ProvisionsFromLocalTip(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	git.RemoteBranches = map[string]bool{"develop": true}
	git.FastForwardErr = errors.New("fatal: refusing to fetch")
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop is provisioned despite the FF failure")
	assert.NotEmpty(t, managed.WorktreePath)
}

func TestImportRepo_SeedsPathSlugFromThePathNotTheSuppliedName(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(
		context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"},
	))
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/widget", Branch: "main", Head: "h1"}}
	prov.Protected = []string{"develop"}

	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover:   func(_ string, _ int) ([]string, error) { return nil, nil },
		RefRunner: func(_ string) defaultbranch.RefRunner {
			return func(_ ...string) (string, bool) { return "", false }
		},
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Name: "My Custom Name", Path: "/root/widget"}, &recordedProgress{})
	require.NoError(t, err)

	assert.Equal(t, "My Custom Name", repo.Name, "the display name is still the supplied one")
	assert.Equal(t, "widget", repo.PathSlug,
		"the on-disk slug is seeded from filepath.Base(repo.Path)")
	require.Len(t, repos.Saved, 1)
	assert.Equal(t, "widget", repos.Saved[0].PathSlug, "the seeded slug is persisted")

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop is provisioned as a managed worktree")
	assert.Equal(t,
		filepath.Join("/crowbar-home", "projects", "proj-1", "widget", "develop", "worktree"),
		managed.WorktreePath,
		"the managed worktree hangs off the path slug, not off the display name")
}

func TestCreate_TheHomeExistsBeforeTheProjectRowIsSaved(t *testing.T) {
	projects, _, ws, _, _, uc := newImport(t)
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
		assert.Empty(t, projects.Saved, "the project row must not be announced before its home exists")
		row := domain.Workspace{ID: in.ID, ProjectID: in.ProjectID, Kind: in.Kind, WorktreePath: in.WorktreePath, Provisioning: domain.WorkspaceProvisioned}
		ws.Created = append(ws.Created, row)
		return row, nil
	}

	_, err := uc.Create(context.Background(), "", "myproject", t.TempDir())
	require.NoError(t, err)
	require.Len(t, ws.Created, 1)
	require.Len(t, projects.Saved, 1)
}

func TestCreate_AFailedProjectSaveDeletesTheHome(t *testing.T) {
	projects, _, ws, _, _, uc := newImport(t)
	projects.SaveErr = errors.New("disk full")

	_, err := uc.Create(context.Background(), "", "myproject", t.TempDir())
	require.ErrorContains(t, err, "disk full")
	require.Len(t, ws.Created, 1)
	assert.Equal(t, []string{ws.Created[0].ID}, ws.Deleted)
}

func TestCreate_InvalidPath_PersistsNothing(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        time.Now,
		Stat:       func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	})

	_, err := uc.Create(context.Background(), "", "P", "/nonexistent")
	require.ErrorIs(t, err, project.ErrFolderNotFound)
	assert.Empty(t, projects.Saved)
	assert.Empty(t, ws.Created)
}

func TestCreate_ProjectSaveError(t *testing.T) {
	projects := mocks.NewProjectStore()
	projects.SaveErr = errors.New("boom")
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        time.Now,
		Stat:       statExists,
	})

	_, err := uc.Create(context.Background(), "", "P", "/root")
	require.Error(t, err)
}

func TestCreate_HomeWorkspaceFails_RollsBackProject(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	ws.CreateErr = errors.New("ws boom")
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        time.Now,
		Stat:       statExists,
	})

	_, err := uc.Create(context.Background(), "", "P", "/root")
	require.Error(t, err)
	assert.Empty(t, projects.Saved, "a failed home workspace must roll the project row back")
}

func TestImport_ProjectHomeWorkspaceFails_RollsBackProject(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
		if in.Kind == domain.WorkspaceKindHome {
			return domain.Workspace{}, errors.New("project home boom")
		}
		return domain.Workspace{ID: in.ID}, nil
	}
	discoverCalled := false
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover: func(string, int) ([]string, error) {
			discoverCalled = true
			return nil, nil
		},
		RefRunner: noRefRunner,
		Now:       func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:      statExists,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.Error(t, err)
	assert.Empty(t, projects.Saved, "a failed project home workspace must roll the project row back")
	assert.False(t, discoverCalled, "repo discovery must never run once the project home has failed")
}

func TestImport_StatGenericError_Surfaces(t *testing.T) {
	uc := newImportUsecase(t, project.ImportDeps{
		Projects:   mocks.NewProjectStore(),
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: mocks.NewWorkspaceRepo(),
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        time.Now,
		Stat:       func(string) (os.FileInfo, error) { return nil, errors.New("permission denied") },
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	require.Error(t, err)
	require.NotErrorIs(t, err, project.ErrFolderNotFound,
		"a genuine stat failure must surface distinctly from a clean not-found")
}

func TestNewImport_NilCrowbarHomePanics(t *testing.T) {
	assert.Panics(t, func() { project.NewImport(project.ImportDeps{}) })
}
