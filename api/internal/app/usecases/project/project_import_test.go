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
	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Discover: func(
			_ string,
			_ int,
		) ([]string, error) {
			// repo.Path must equal the main worktree's path so samePath identifies
			// it as the default; the worktree fixtures below use "/repoA".
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
		// Deterministic managed-worktree root so protected-branch worktree paths
		// are predictable in assertions.
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
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
	// main is the home's branch (held-by-home → placeholder); develop is remote-only
	// and unheld (→ managed worktree).
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

	// project home (Kind=home) + repo home (IsDefault, stays on main, NOT detached)
	// + a main PLACEHOLDER (held by the home) + a develop MANAGED worktree.
	require.Len(t, ws.Created, 4)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	// The project-home workspace keeps the user's checkout (project.Path), never a
	// Crowbar-managed .home leaf (spec §3.9 adopted-home law, Task 3b decision).
	assert.Equal(t, "/root", ws.Created[0].WorktreePath)

	home := ws.Created[1]
	assert.True(t, home.IsDefault, "the repo home is the default workspace")
	// The adopted repo home stays the user's real checkout (repo.Path), never a
	// Crowbar-managed .home leaf (spec §3.9 adopted-home law, Task 3b decision).
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
	// The managed worktree lands at the human-readable derived path
	// <home>/projects/<project>/<slug>/<branch>/worktree (spec §3.9; the
	// trailing "worktree" leaf makes <slug>/<branch> a workspace root sibling
	// of "chats", spec §3.5); with no reachable remote in tests the slug falls
	// back to the repo name "repoA".
	assert.Equal(t,
		filepath.Join("/crowbar-home", "projects", project.ID, "repoA", "develop", "worktree"),
		managed.WorktreePath)
}

// TestImport_SkipsNonProtectedLocalWorktrees pins the user-requested rule: on
// import, only the repo's main worktree and protected remote branches are
// auto-imported. Other local worktrees (feature/spike/agent checkouts) are NOT
// turned into workspaces — the user adds those explicitly.
func TestImport_SkipsNonProtectedLocalWorktrees(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},                // main (protected) → adopted
		{Path: "/repoA/wt1", Branch: "feature/x", Head: "h2"},          // skip
		{Path: "/repoA/wt2", Branch: "spike/y", Head: "h3"},            // skip
		{Path: "/repoA/wt3", Branch: "worktree-agent-abc", Head: "h4"}, // skip
	}
	prov.Protected = []string{"develop", "main"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	// develop is the home's branch (held-by-home → placeholder); main is unheld
	// (→ managed). Non-protected local worktrees are still not imported.
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
	// A worktree-list failure aborts adoption (no workspaces), so the repo is
	// rolled back rather than orphaned; the multi-repo import still succeeds.
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
	// A protected-branches failure is SOFT: without protected info Crowbar cannot
	// know whether to detach or which managed worktrees to create, so it imports
	// the repo home alone (kept, not rolled back) and provisions no protected
	// worktrees. The home keeps its branch — it is never detached blindly.
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
	// When the repo home is checked out on a NON-default branch, Crowbar leaves the
	// directory alone: the home keeps that branch and is never detached. The repo
	// reports no protected branches, so the resolved default branch ("main") is the
	// one that gets locked — and because it is unheld here (the home sits on
	// "feature"), it becomes its own managed worktree rather than a placeholder.
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
		{Path: "/repoA", Branch: "main", Head: "h1"},      // main (protected) → home detaches + managed wt
		{Path: "/repoA/wt", Branch: "", Head: "detached"}, // unrelated non-main worktree → not imported
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
	// project home + repo home (main) + the "main" placeholder (main is the
	// resolved default, held by the home). The prunable "dead" worktree is skipped.
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
	// A workspace-create failure inside importOneRepo is tolerated at the Import
	// level (the whole multi-repo import does not fail), but the repo whose worktree
	// adoption failed is ROLLED BACK rather than persisted as an orphaned,
	// unnavigable row (one with no workspaces).
	// The project-level home workspace create must still succeed so Import proceeds.
	projects, repos, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.RepoID != "" {
			// Repo-scoped workspace creation fails — adoption aborts for this repo.
			return domain.Workspace{}, errors.New("ws boom")
		}
		// Home workspace — succeed and record it.
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
	// A local repo icon (favicon.svg) on disk must be copied into the
	// entity-scoped icon path <home>/projects/<P>/<R>/icon, and the repo row
	// must record AvatarHasIcon=true.
	home := t.TempDir()
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "favicon.svg"), []byte("<svg>icon</svg>"), 0o644))

	repos := mocks.NewRepositoryStore()
	// A real repo always has a main worktree, so adoption yields >=1 workspace and
	// the repo is kept (a repo with no adoptable worktree is now rolled back).
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := project.NewImport(project.ImportDeps{
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
	// No local icon: import must best-effort fetch the GitHub owner avatar
	// bytes and write them to the entity icon path, setting AvatarHasIcon=true.
	home := t.TempDir()
	repoDir := t.TempDir() // no icon files inside

	repos := mocks.NewRepositoryStore()
	fetched := false
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := project.NewImport(project.ImportDeps{
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
	// No local icon AND the github fetch yields nothing: the repo must still
	// be saved with AvatarHasIcon=false (generated label/color avatar) and the
	// import must NOT fail.
	home := t.TempDir()
	repoDir := t.TempDir() // no icon files

	repos := mocks.NewRepositoryStore()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := project.NewImport(project.ImportDeps{
		Projects:    mocks.NewProjectStore(),
		Repos:       repos,
		Workspaces:  mocks.NewWorkspaceRepo(),
		Git:         git,
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

// TestImport_ProvisionsManagedWorktreesForProtectedBranches: home on protected
// main, develop also protected but unheld. Under the recovery model the home is
// adopted IN PLACE on main (never detached), main becomes exactly ONE placeholder
// (locked, empty WorktreePath, heldByPath == repo folder), and develop (free) gets
// its own Crowbar-managed locked worktree.
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

	// main is held by the home → a placeholder (locked, no worktree, holder recorded).
	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["main"].Status, "main is a locked placeholder")
	assert.Empty(t, byBranch["main"].WorktreePath, "the home-held main is a placeholder with no worktree")
	assert.Equal(t, "/repoA", byBranch["main"].HeldByPath, "the placeholder records the home as holder")

	// develop is unheld → its own managed worktree (never a stub at the repo folder).
	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["develop"].Status, "develop is locked")
	assert.NotEqual(t, "/repoA", byBranch["develop"].WorktreePath, "develop gets a managed worktree, not the repo folder")
	// Human-readable derived path: <slug>/<branch>/worktree with slug = repo name
	// "repoA" (no reachable remote in tests) and a trailing "worktree" leaf (the
	// workspace-root split, spec §3.5) — never a UUID-keyed path.
	assert.Contains(t, byBranch["develop"].WorktreePath, "/repoA/develop")
	assert.True(t, strings.HasSuffix(byBranch["develop"].WorktreePath, "/worktree"),
		"managed worktree path must end in the workspace-root's worktree leaf")
	assert.Empty(t, byBranch["develop"].HeldByPath, "a managed worktree has no holder")
}

// TestImport_DefaultProtectedBranch_HomeInPlaceAndPlaceholder: the checked-out
// default ("develop") is protected. The repo home is adopted IN PLACE (keeps
// develop, never detached) and develop becomes exactly ONE placeholder (locked,
// empty WorktreePath, heldByPath == repo folder) — no duplicate, no stub, no
// managed worktree seized from under the user's live checkout.
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

// TestImport_NoProtectedBranches_DefaultBranchLockedManagedWorktree proves that a
// repo the provider reports as having NO protected branches at all still gets its
// default branch materialised as a locked, Crowbar-managed worktree. This is the
// base-branch-locked UX contract: a repo with no GitHub protection rules (or where
// gh is authed but returns an empty set) must not import with only its home and
// nothing lockable. The default branch (origin/main per the newImport RefRunner)
// is unheld here, so it becomes its own managed worktree, not a placeholder.
func TestImport_NoProtectedBranches_DefaultBranchLockedManagedWorktree(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// Home sits on a feature branch; the default branch "main" is unheld and free.
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{} // provider reports zero protected branches

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

func TestImport_RemoteProtectedBranch_FetchedAndForkPointRecorded(t *testing.T) {
	// A protected branch that exists on origin is fetched before being checked out
	// into its managed worktree, and its fork point is recorded from RevParse.
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"release"}
	git.RemoteBranches = map[string]bool{"release": true}
	// Fork point is resolved via refs/heads/<branch> (not the bare name) to avoid
	// tag-name precedence in git rev-parse.
	git.RevParseShas = map[string]string{"refs/heads/release": "deadbeef"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	assert.Contains(t, git.FastForwardedBranches, "release", "a remote protected branch is fast-forwarded before checkout")
	require.Len(t, git.WorktreeAdds, 1)
	assert.Equal(t, "release", git.WorktreeAdds[0].Branch)

	var release *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			release = &ws.Created[i]
		}
	}
	require.NotNil(t, release)
	assert.Equal(t, "deadbeef", release.ForkPointSha, "managed worktree records the resolved fork point")
}

func TestImport_LocalOnlyProtectedBranch_NotFetched(t *testing.T) {
	// A protected branch with no origin counterpart is checked out from the local
	// ref without a fetch.
	_, _, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"master"} // RemoteBranches empty → local-only

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.NotContains(t, git.FastForwardedBranches, "master", "a local-only protected branch is not fast-forwarded")
	require.Len(t, git.WorktreeAdds, 1)
	assert.Equal(t, "master", git.WorktreeAdds[0].Branch)
}

func TestImport_ProtectedWorktreeFailure_IsBestEffort(t *testing.T) {
	// A failure provisioning ONE protected branch's worktree is logged and skipped:
	// the repo is kept (it already has its home) and the other protected branches
	// still get their managed worktrees.
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
	// If the workspace ROW fails to persist after the managed worktree was created
	// on disk, the orphaned worktree is removed so a retry is clean.
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.Protected {
			return domain.Workspace{}, errors.New("row boom") // the managed protected row fails
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault, WorktreePath: in.WorktreePath}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err) // best-effort: import still succeeds
	require.Len(t, git.WorktreeAdds, 1, "the managed worktree was created on disk")
	require.Len(t, git.WorktreeRemoves, 1, "the orphaned worktree is cleaned up after the row failed")
	assert.Equal(t, git.WorktreeAdds[0].Path, git.WorktreeRemoves[0], "cleanup removes the same worktree path")
}

func TestImport_DetachFailure_DegradesAndStillImports(t *testing.T) {
	// If detaching the home off its protected branch fails (mid-merge/rebase, or an
	// unborn branch in a fresh repo), the import must NOT fail: the home is kept on
	// its branch and the repo is still imported (best-effort degrade).
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov.Protected = []string{"main"}
	git.DetachErr = errors.New("fatal: cannot switch branch while merging")
	// The branch stays checked out in the home, so its managed worktree add fails too.
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

// TestImport_HomeRowFailure_NoDetachNoReattach: if the repo home row fails to
// persist, the user's real checkout must be left exactly as it was. Under the
// recovery model the home is adopted IN PLACE (never detached), so there is no
// detach to undo and no reattach to perform. The per-repo failure is tolerated by
// the bulk import.
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
	require.NoError(t, err) // the per-repo failure is tolerated by the bulk import
	assert.Empty(t, git.Detached, "the home is adopted in place, so nothing is ever detached")
	assert.Empty(t, git.CheckedOut, "no reattach: there was no detach to undo")
}

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
	uc := project.NewImport(project.ImportDeps{
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

	r1, err := uc.ImportRepo(context.Background(), "p1", "/root/repoA")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	wsCount := len(ws.Created)

	r2, err := uc.ImportRepo(context.Background(), "p1", "/root/repoA")
	require.NoError(t, err)
	assert.Equal(t, r1.ID, r2.ID, "re-add returns the already-imported repo")
	assert.Len(t, repos.Saved, 1, "no duplicate repository row")
	assert.Len(t, ws.Created, wsCount, "no duplicate workspaces")
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
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})

	repo, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.NoError(t, err)

	require.Len(t, repos.Saved, 1)
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, "proj-1", repo.ProjectID)
	assert.Equal(t, repos.Saved[0].ID, repo.ID)

	// ImportRepo (add-repo-to-existing-project) creates no project home; it adopts
	// the repo home IN PLACE (keeps develop, not detached) plus develop as a locked
	// PLACEHOLDER (held by the home) — never a stub or a seized managed worktree.
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

// TestImportRepo_AdoptionFailure_RollsBackRepo proves pass-6: the single-repo
// ImportRepo path propagates the adoption error AND rolls back the repo row, so a
// user retrying a failed add cannot accumulate orphaned, workspace-less repos.
func TestImportRepo_AdoptionFailure_RollsBackRepo(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(
		context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"},
	))
	git.WorktreeListErr = errors.New("wt boom")

	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
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

	_, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.Error(t, err, "ImportRepo must propagate the adoption failure")
	assert.Empty(t, repos.Saved,
		"the repo row must be rolled back so retries cannot accumulate orphaned repos")
	assert.Empty(t, ws.Created)
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
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := project.NewImport(project.ImportDeps{
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

	// repoA's adoption failed, so its repo row is rolled back; only repoB (which
	// adopted a workspace) persists — no orphaned, workspace-less row remains.
	require.Len(t, repos.Saved, 1, "the failed repo must be rolled back; only repoB persists")
	assert.Equal(t, "repoB", repos.Saved[0].Name)

	// The project home is provisioned first, then repoB's home (branch "main"), then
	// repoB's default-branch "main" placeholder (main is the resolved default, held
	// by repoB's home). repoA failed before creating any workspace.
	require.Len(t, ws.Created, 3, "project home + repoB home + repoB default-branch placeholder")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.Equal(t, "main", ws.Created[1].Branch)
}

// TestCreate_ProvisionesHomeWorkspace proves that Create auto-provisions a
// project-level home workspace (Kind=WorkspaceKindHome, WorktreePath=project.Path)
// immediately after saving the project row.
func TestCreate_ProvisionesHomeWorkspace(t *testing.T) {
	_, _, ws, _, _, uc := newImport(t)

	dir := t.TempDir()
	_, err := uc.Create(context.Background(), "myproject", dir)
	require.NoError(t, err)

	require.Len(t, ws.Created, 1)
	require.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	require.Equal(t, dir, ws.Created[0].WorktreePath)
}

// TestImport_ProvisionesHomeWorkspace proves that Import auto-provisions a
// project-level home workspace before discovering repos.
func TestImport_ProvisionesHomeWorkspace(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main"}

	_, err := uc.Import(context.Background(), "myproject", "/root")
	require.NoError(t, err)

	// First entry must be the home workspace.
	require.GreaterOrEqual(t, len(ws.Created), 1)
	homeWS := ws.Created[0]
	require.Equal(t, domain.WorkspaceKindHome, homeWS.Kind)
	require.Equal(t, "/root", homeWS.WorktreePath)
	require.Empty(t, homeWS.RepoID, "home workspace must not be repo-scoped")
}

// TestImport_HomeHeldProtectedBranch_YieldsSinglePlaceholder is the exact
// char2cs/asynx case: home on develop, protected develop+master. The home is
// adopted IN PLACE (not detached), develop becomes exactly ONE placeholder
// (locked, empty WorktreePath, heldByPath == repoPath), master (unheld) becomes
// a managed worktree — never two develop rows (spec §3.5/B5).
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

// TestImport_ExternalHolder_YieldsPlaceholder: a protected branch held by a live
// worktree OUTSIDE the crowbar home becomes a placeholder recording that path.
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

// TestImport_DeadRegistrationPruned_ProvisionsCleanly: a protected branch whose
// only "holder" is a dead worktree registration is freed by the prune-before in
// holder.Resolve and provisions a managed worktree — never dropped. The mock
// GitEngine drops dead regs on WorktreePrune (see step 3).
func TestImport_DeadRegistrationPruned_ProvisionsCleanly(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	// A dead registration: develop registered at a now-deleted dir; prune reaps it.
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

// TestImport_ParentFetchFails_ProvisionsFromLocalTip: FastForwardBranch failing
// (offline / refused) must NOT skip the branch — the worktree is added from the
// local tip (best-effort FF, matching addWorktree).
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
