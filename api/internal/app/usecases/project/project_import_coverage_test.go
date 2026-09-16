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

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// noRefRunner is the RefRunnerFactory every test below that never resolves a
// default branch via git plumbing can share.
func noRefRunner(string) defaultbranch.RefRunner {
	return func(...string) (string, bool) { return "", false }
}

// --- Create(): the lightweight variant's own error branches ---

// TestCreate_InvalidPath_PersistsNothing proves Create validates the path
// BEFORE persisting anything, exactly like Import.
func TestCreate_InvalidPath_PersistsNothing(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	uc := newImportUsecase(project.ImportDeps{
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

	_, err := uc.Create(context.Background(), "P", "/nonexistent")
	require.ErrorIs(t, err, project.ErrFolderNotFound)
	assert.Empty(t, projects.Saved)
	assert.Empty(t, ws.Created)
}

func TestCreate_ProjectSaveError(t *testing.T) {
	projects := mocks.NewProjectStore()
	projects.SaveErr = errors.New("boom")
	uc := newImportUsecase(project.ImportDeps{
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

	_, err := uc.Create(context.Background(), "P", "/root")
	require.Error(t, err)
}

// TestCreate_HomeWorkspaceFails_RollsBackProject proves Create's own
// createHomeWorkspace failure rolls the just-saved project row back, mirroring
// Import's equivalent rollback.
func TestCreate_HomeWorkspaceFails_RollsBackProject(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	ws.CreateErr = errors.New("ws boom")
	uc := newImportUsecase(project.ImportDeps{
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

	_, err := uc.Create(context.Background(), "P", "/root")
	require.Error(t, err)
	assert.Empty(t, projects.Saved, "a failed home workspace must roll the project row back")
}

// --- Import(): the project-level home workspace's own error branch ---

// TestImport_ProjectHomeWorkspaceFails_RollsBackProject proves a failure
// creating the PROJECT-level home workspace (Kind=Home, run before any repo is
// even discovered) rolls the project row back — distinct from
// TestImport_HomeRowFailure_NoDetachNoReattach, which fails the REPO home.
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
	uc := newImportUsecase(project.ImportDeps{
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

// --- importOneRepo: the CheckRepoImportable refusal reached through the real
// create/import path (as opposed to the standalone CheckRepoImportable tests) ---

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

	uc := newImportUsecase(project.ImportDeps{
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

	_, err := uc.ImportRepo(context.Background(), "proj-2", "", "/shared/repo")

	require.ErrorIs(t, err, project.ErrRepoAlreadyImported)
	assert.Contains(t, err.Error(), "First", "the refusal names the project that already owns the folder")
	require.Len(t, repos.Saved, 1, "no second row for the same folder under a different project")
}

// --- writeRepoIcon / provisionProtectedWorktrees: shared CrowbarHome guard ---

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
	uc := newImportUsecase(project.ImportDeps{
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
	uc := newImportUsecase(project.ImportDeps{
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

// --- resolveIconBytes: the FetchAvatarBytes ERROR path (distinct from the
// "no data, no error" degrade already covered) ---

func TestImport_AvatarFetchError_LeavesGeneratedAvatar(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()

	repos := mocks.NewRepositoryStore()
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	uc := newImportUsecase(project.ImportDeps{
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

// --- existingRepo: FindAll error degrades to "not yet imported" ---

func TestImportRepo_ExistingRepoLookupError_StillImports(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	repos.FindErr = errors.New("db down")
	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "main", Head: "h1"}}

	uc := newImportUsecase(project.ImportDeps{
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

	repo, err := uc.ImportRepo(context.Background(), "proj-1", "", "/root/repoA")
	require.NoError(t, err, "a read failure resolving prior imports must not block a legitimate import")
	assert.Equal(t, "repoA", repo.Name)
}

// --- mainWorktreeBranch: no worktree entry matches the repo path at all ---

func TestImport_NoWorktreeMatchesRepoPath_HomeHasEmptyBranch(t *testing.T) {
	_, _, ws, git, prov, uc := newImportForCoverage(t)
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

// --- provisionProtectedBranchWorktree: holder.Resolve failing specifically
// during PROVISIONING (after the home has already adopted successfully) ---

// TestImport_HolderResolveFailsDuringProvisioning_SkipsThatBranchOnly proves a
// git failure resolving a protected branch's holder (here, on the SECOND
// WorktreeList call — the first, made by home adoption, must still succeed) is
// logged and skipped for that branch alone; the repo (already home-adopted) is
// kept.
func TestImport_HolderResolveFailsDuringProvisioning_SkipsThatBranchOnly(t *testing.T) {
	_, repos, ws, git, prov, uc := newImportForCoverage(t)
	calls := 0
	git.WorktreeListFn = func(repoPath string) ([]gitengine.WorktreeEntry, error) {
		calls++
		if calls == 1 {
			return []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}, nil
		}
		return nil, errors.New("holder resolve boom")
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a holder-resolve failure for one branch must not fail the import")
	require.Len(t, repos.Saved, 1, "the repo is kept — it already has its home")
	for _, w := range ws.Created {
		assert.NotEqual(t, "develop", w.Branch, "the branch whose holder could not be resolved is skipped")
	}
}

// --- FreePathBranch: a branch name that escapes the managed tree must never
// be silently accepted ---

func TestImport_ProtectedBranchNameEscapesManagedTree_IsSkipped(t *testing.T) {
	_, repos, ws, git, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"../../../../../../../../tmp/evil"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a path-escaping branch name must be skipped, not fail the whole import")
	require.Len(t, repos.Saved, 1)
	for _, w := range ws.Created {
		assert.NotContains(t, w.WorktreePath, "/tmp/evil",
			"a traversal branch name must never derive a worktree path outside the managed tree")
	}
	assert.Empty(t, git.WorktreeAdds, "no worktree is ever added for the escaping branch")
}

// --- createPlaceholderWorkspace: its OWN Create call failing ---

// TestImport_PlaceholderWorkspaceCreateFails_IsLoggedAndSkipped proves that
// when a protected branch is HELD (so createPlaceholderWorkspace runs) and
// that placeholder's own row-create fails, the failure is logged and the
// branch is skipped — it does not roll back the repo, which already has its
// home.
func TestImport_PlaceholderWorkspaceCreateFails_IsLoggedAndSkipped(t *testing.T) {
	_, repos, ws, _, prov, uc := newImportForCoverage(t)
	// "main" is held BY THE HOME (git.Worktrees has home on "main"), so it goes
	// through createPlaceholderWorkspace rather than the free-branch path.
	prov.Protected = []string{"main"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
		// Let both the project-level home (Kind=Home) and the repo home
		// (IsDefault) through; only the placeholder create (neither) fails.
		if in.Kind == domain.WorkspaceKindHome || in.IsDefault {
			created := domain.Workspace{ID: in.ID, IsDefault: in.IsDefault, Kind: in.Kind, Branch: in.Branch}
			ws.Created = append(ws.Created, created)
			return created, nil
		}
		return domain.Workspace{}, errors.New("placeholder row boom")
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a failed placeholder create must be best-effort, not fail the import")
	require.Len(t, repos.Saved, 1, "the repo is kept — it already has its home")
	var home *domain.Workspace
	for i := range ws.Created {
		w := ws.Created[i]
		if w.Branch == "main" && !w.IsDefault {
			t.Fatal("no placeholder row should exist for the branch whose create failed")
		}
		if w.IsDefault {
			home = &ws.Created[i]
		}
	}
	require.NotNil(t, home, "the home adoption itself must still have succeeded")
}

// --- addProtectedWorktree: the non-essential post-add RevParse failing ---

func TestImport_ProtectedWorktree_RevParseAfterAddFails_ForkPointEmptyButKept(t *testing.T) {
	_, repos, ws, git, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"develop"} // not on origin (RemoteTrackingBranches unset) -> plain WorktreeAdd path
	git.RevParseErr = errors.New("rev-parse boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "the worktree is still valid and kept even though the fork point could not be read")
	assert.Empty(t, managed.ForkPointSha)
}

// --- addProtectedWorktreeFromOrigin: FetchRef best-effort, and its own
// WorktreeAddAtRef/SetUpstream error branches ---

func TestImport_ProtectedWorktreeFromOrigin_FetchFails_StillChecksOutFromLocalRef(t *testing.T) {
	_, repos, ws, git, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"release"}
	git.RemoteTrackingBranches = map[string]bool{"release": true}
	git.FetchRefErr = errors.New("fetch boom")
	git.RevParseShas = map[string]string{"origin/release": "originsha"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a best-effort origin refresh failure must not skip the branch")
	require.Len(t, repos.Saved, 1)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "the branch is still checked out from the local remote-tracking ref")
	require.Len(t, git.WorktreeAddAtRefs, 1)
}

func TestImport_ProtectedWorktreeFromOrigin_SetUpstreamFails_IsBestEffort(t *testing.T) {
	_, repos, ws, git, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"release"}
	git.RemoteTrackingBranches = map[string]bool{"release": true}
	git.RevParseShas = map[string]string{"origin/release": "originsha"}
	git.SetUpstreamErr = errors.New("set-upstream boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "content is already correct; a failed upstream link must not drop the branch")
	assert.Equal(t, "originsha", managed.ForkPointSha)
}

// --- validateImportPath: a Stat failure that is NOT "does not exist" ---

func TestImport_StatGenericError_Surfaces(t *testing.T) {
	uc := newImportUsecase(project.ImportDeps{
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

// --- gitRemoteURL: the REAL success path, which needs an actual git repo ---

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

	uc := newImportUsecase(project.ImportDeps{
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

	repo, err := uc.ImportRepo(context.Background(), "proj-1", "", repoDir)
	require.NoError(t, err)
	assert.Equal(t, remoteURL, repo.RemoteURL, "the repo's real origin remote must be recorded")
	require.Len(t, repos.Saved, 1)
	assert.Equal(t, remoteURL, repos.Saved[0].RemoteURL)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
}

// --- siblingWorktreePaths: the real, successful non-empty listing ---

// TestImport_ProtectedBranchName_DisambiguatesAgainstRealSiblingDirectory
// proves siblingWorktreePaths' success path actually finds a pre-existing
// sibling: importing a protected branch whose derived directory name collides
// case-insensitively with an ALREADY-EXISTING directory on disk (e.g. left
// over from an earlier session) lands at the next free variant instead of
// colliding with it — mirroring the analogous worktree.go create-path
// regression, but for the project-import protected-branch provisioning path.
func TestImport_ProtectedBranchName_DisambiguatesAgainstRealSiblingDirectory(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir() // no real .git -> RemoteURL is "", slug falls back to filepath.Base(repoDir)
	slug := filepath.Base(repoDir)

	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))

	// A sibling directory already sits at the exact path "develop" would derive
	// to, differing only in case — as if left over from an earlier session.
	slugDir := filepath.Join(home, "projects", "proj-1", slug)
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "Develop"), 0o755))

	git := mocks.NewGitEngine()
	// Home sits on "main" (unprotected here), so only "develop" is provisioned —
	// free, and thus routed through the on-disk managed-worktree path whose
	// sibling scan must see the pre-existing "Develop" directory.
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	ws := mocks.NewWorkspaceRepo()

	uc := newImportUsecase(project.ImportDeps{
		Projects:    projects,
		Repos:       repos,
		Workspaces:  ws,
		Git:         git,
		Provider:    &protectedProvider{branches: []string{"develop"}},
		CrowbarHome: func() (string, error) { return home, nil },
		Discover:    func(string, int) ([]string, error) { return nil, nil },
		RefRunner:   noRefRunner,
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
	})

	_, err := uc.ImportRepo(context.Background(), "proj-1", "", repoDir)
	require.NoError(t, err)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop must still be provisioned")
	assert.Contains(t, managed.WorktreePath, "develop-2",
		"the case-colliding sibling must push the new directory to the next free variant")
	assert.DirExists(t, filepath.Join(slugDir, "Develop"), "the pre-existing sibling is left untouched")
}

// protectedProvider is a minimal ImportProviderEngine reporting a fixed
// protected-branch list.
type protectedProvider struct {
	branches []string
}

func (p *protectedProvider) ProtectedBranches(context.Context, string) ([]string, error) {
	return p.branches, nil
}

// newImportForCoverage builds an ImportUsecase against repo "/repoA" (the home
// checked out on "main"), matching the newImport() fixture's shape in
// project_import_test.go, for tests in THIS file that need their own git/prov
// handles without colliding with that helper's exact field set.
func newImportForCoverage(
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
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov := mocks.NewProviderEngine()
	uc := newImportUsecase(project.ImportDeps{
		Projects:    projects,
		Repos:       repos,
		Workspaces:  ws,
		Git:         git,
		Provider:    prov,
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Discover:    func(string, int) ([]string, error) { return []string{"/repoA"}, nil },
		RefRunner:   noRefRunner,
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
	})
	return projects, repos, ws, git, prov, uc
}
