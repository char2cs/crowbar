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

	// Workspace model: the project home (Kind=home) is provisioned first; then the
	// repo home (IsDefault) — detached off the protected "main" so that branch is
	// free — then "main" as its OWN Crowbar-managed locked worktree. The local
	// "feature" worktree is non-protected, so it is left for the user to add.
	require.Len(t, ws.Created, 3)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind,
		"first created workspace must be the project home workspace")

	home := ws.Created[1]
	assert.True(t, home.IsDefault, "the repo home is the default workspace")
	assert.Equal(t, "/repoA", home.WorktreePath, "the repo home stays the repo folder")
	assert.Empty(t, home.Branch, "the repo home is detached off the protected branch")
	assert.NotEqual(t, domain.WorkspaceStatusLocked, home.Status, "the repo home is never locked")
	assert.Contains(t, git.Detached, "/repoA", "the repo home is detached off its protected branch")

	mainWs := ws.Created[2]
	assert.Equal(t, "main", mainWs.Branch)
	assert.Equal(t, domain.WorkspaceStatusLocked, mainWs.Status, "the protected branch is a locked workspace")
	assert.False(t, mainWs.IsDefault)
	assert.NotEqual(t, "/repoA", mainWs.WorktreePath,
		"a protected branch gets a Crowbar-managed worktree, never the repo folder")
	assert.Contains(t, mainWs.WorktreePath, "/crowbar-home/projects/")
	assert.Contains(t, mainWs.WorktreePath, "/worktree")
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

	// Every protected branch (develop, main) becomes its OWN managed worktree;
	// the repo home is detached off the protected default; non-protected local
	// worktrees are not imported.
	managed := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			managed[w.Branch] = w
		}
	}
	assert.Contains(t, managed, "develop", "protected branch gets a managed worktree")
	assert.Contains(t, managed, "main", "protected branch gets a managed worktree")
	assert.NotContains(t, managed, "feature/x", "non-protected local worktree is NOT imported")
	assert.NotContains(t, managed, "spike/y", "non-protected local worktree is NOT imported")
	assert.NotContains(t, managed, "worktree-agent-abc", "non-protected local worktree is NOT imported")
	for _, b := range []string{"develop", "main"} {
		assert.Equal(t, domain.WorkspaceStatusLocked, managed[b].Status, b+" is a locked workspace")
		assert.NotEqual(t, "/repoA", managed[b].WorktreePath, b+" gets a managed worktree, not the repo folder")
	}
	assert.Len(t, ws.Created, 4, "project home + repo home + develop + main managed worktrees")
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
	// When the repo home is checked out on a NON-protected branch, Crowbar leaves
	// the directory alone: the home keeps that branch, is not detached, and no
	// protected worktrees are created. The home carries no fork point (it is the base).
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h2"}}
	prov.Protected = nil

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 2, "project home + repo home only")
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "feature", home.Branch, "a non-protected home keeps its branch")
	assert.Empty(t, home.ForkPointSha)
	assert.Empty(t, git.Detached, "no detach for a non-protected home branch")
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
	require.Len(t, ws.Created, 2)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)
	assert.Equal(t, "main", ws.Created[1].Branch)
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

func TestImport_ProvisionsManagedWorktreesForProtectedBranches(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// main is the checked-out (protected) branch; develop is protected with no
	// local worktree. BOTH must end up as their own Crowbar-managed locked
	// worktrees (never a stub at the repo folder).
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	require.Len(t, ws.Created, 4, "project home + repo home + main + develop managed worktrees")
	managed := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			managed[w.Branch] = w
		}
	}
	require.Contains(t, managed, "main")
	require.Contains(t, managed, "develop")
	for _, b := range []string{"main", "develop"} {
		assert.Equal(t, domain.WorkspaceStatusLocked, managed[b].Status, b+" is locked")
		assert.NotEqual(t, "/repoA", managed[b].WorktreePath, b+" gets a managed worktree, not the repo folder")
		assert.Contains(t, managed[b].WorktreePath, "/worktree")
	}
	assert.Contains(t, git.Detached, "/repoA", "the repo home detaches off the protected main")
}

func TestImport_DefaultProtectedBranch_HomeDetachedAndManaged(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// The checked-out default ("develop") is protected: the repo home detaches off
	// it and develop gets exactly ONE managed worktree (no duplicate, no stub).
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3, "project home + repo home (detached) + develop managed worktree")

	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Empty(t, home.Branch, "home detached off the protected develop")
	assert.Equal(t, "/repoA", home.WorktreePath)

	develop := ws.Created[2]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.NotEqual(t, "/repoA", develop.WorktreePath)

	count := 0
	for _, w := range ws.Created {
		if w.Branch == "develop" {
			count++
		}
	}
	assert.Equal(t, 1, count, "develop appears exactly once (managed worktree, not also the home)")
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

	assert.Contains(t, git.FetchedRefs, "release", "a remote protected branch is fetched before checkout")
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
	assert.NotContains(t, git.FetchedRefs, "master", "a local-only protected branch is not fetched")
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

func TestImport_HomeRowFailureAfterDetach_ReattachesRepo(t *testing.T) {
	// If the home workspace row fails to persist AFTER the home was detached off a
	// protected branch, the user's real repo must be re-attached — never left on a
	// detached HEAD with no DB state.
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
	assert.Contains(t, git.Detached, "/repoA", "the home was detached off the protected branch")
	require.Len(t, git.CheckedOut, 1, "the repo must be re-attached after the home row failed")
	assert.Equal(t, "/repoA", git.CheckedOut[0].Path)
	assert.Equal(t, "develop", git.CheckedOut[0].Branch, "re-attached onto the original branch")
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
	// the repo home (detached off the protected default) plus develop as a managed
	// locked worktree.
	require.Len(t, ws.Created, 2, "repo home (detached) + develop managed worktree")
	home := ws.Created[0]
	assert.True(t, home.IsDefault)
	assert.Empty(t, home.Branch, "repo home detached off the protected develop")
	assert.Equal(t, repo.ID, home.RepoID)
	develop := ws.Created[1]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, repo.ID, develop.RepoID)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.NotEqual(t, "/root/repoA", develop.WorktreePath, "develop gets a managed worktree")
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

	// The home workspace is provisioned first, then repoB's workspace (branch "main").
	require.Len(t, ws.Created, 2, "home workspace + repoB workspace must be adopted")
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
