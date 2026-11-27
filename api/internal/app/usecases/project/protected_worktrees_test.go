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

func noRefRunner(string) defaultbranch.RefRunner {
	return func(...string) (string, bool) { return "", false }
}

func TestImport_HolderResolveFailsDuringProvisioning_SkipsThatBranchOnly(t *testing.T) {
	repos, ws, git, prov, uc := newImportForCoverage(t)
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

func TestImport_ProtectedBranchNameEscapesManagedTree_IsSkipped(t *testing.T) {
	repos, ws, git, prov, uc := newImportForCoverage(t)
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

func TestImport_PlaceholderWorkspaceCreateFails_IsLoggedAndSkipped(t *testing.T) {
	repos, ws, _, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"main"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
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

func TestImport_ProtectedWorktree_RevParseAfterAddFails_ForkPointEmptyButKept(t *testing.T) {
	repos, ws, git, prov, uc := newImportForCoverage(t)
	prov.Protected = []string{"develop"}
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

func TestImport_ProtectedWorktreeFromOrigin_FetchFails_StillChecksOutFromLocalRef(t *testing.T) {
	repos, ws, git, prov, uc := newImportForCoverage(t)
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
	repos, ws, git, prov, uc := newImportForCoverage(t)
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

func TestImport_ProtectedBranchName_DisambiguatesAgainstRealSiblingDirectory(t *testing.T) {
	home := t.TempDir()
	repoDir := t.TempDir()
	slug := filepath.Base(repoDir)

	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))

	slugDir := filepath.Join(home, "projects", "proj-1", slug)
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "Develop"), 0o755))

	git := mocks.NewGitEngine()
	git.Worktrees = []gitengine.WorktreeEntry{{Path: repoDir, Branch: "main", Head: "h1"}}
	ws := mocks.NewWorkspaceRepo()

	uc := newImportUsecase(t, project.ImportDeps{
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

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ProjectID: "proj-1", Path: repoDir}, &recordedProgress{})
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

type protectedProvider struct {
	branches []string
}

func newImportForCoverage(
	t *testing.T,
) (
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
	uc := newImportUsecase(t, project.ImportDeps{
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
	return repos, ws, git, prov, uc
}

func TestImport_ProtectedRowFailure_CleanupWorktreeRemoveAlsoFails(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main"}
	git.WorktreeRemoveErr = errors.New("remove boom")
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.Protected {
			return domain.Workspace{}, errors.New("row boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault, WorktreePath: in.WorktreePath, Provisioning: domain.WorkspaceProvisioned}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a failed best-effort worktree cleanup must not fail the import")
	require.Len(t, repos.Saved, 1, "the repo already has its home and is kept")
	require.Len(t, git.WorktreeAdds, 1, "the managed worktree was attempted")
}

func TestImport_RemoteProtectedBranch_WorktreeAddAtRefError_IsBestEffort(t *testing.T) {
	_, repos, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"release"}
	git.RemoteTrackingBranches = map[string]bool{"release": true}
	git.WorktreeAddErrByBranch = map[string]error{"release": errors.New("checkout boom")}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err, "a failed protected-branch checkout is best-effort, not a batch failure")
	require.Len(t, repos.Saved, 1)
	for _, w := range ws.Created {
		assert.NotEqual(t, "release", w.Branch, "the branch whose checkout failed gets no row")
	}
}

func (p *protectedProvider) ProtectedBranches(context.Context, string) ([]string, error) {
	return p.branches, nil
}

func TestImportRepo_AProtectedWorktreeFailureIsAFailedItem(t *testing.T) {
	projects, repos, ws, git, prov, uc := newImport(t)
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main", "develop"}
	git.WorktreeAddErrByBranch = map[string]error{"main": errors.New("add boom")}
	progress := &recordedProgress{}

	repo, err := uc.ImportRepo(context.Background(), project.RepoImport{ID: "r-1", ProjectID: "proj-1", Path: "/root/repoA"}, progress)
	require.NoError(t, err)
	assert.Equal(t, "r-1", repo.ID, "the repo is created under the caller's id")
	require.Len(t, repos.Saved, 1, "the repo is kept")

	require.Len(t, progress.outcomes, 2)
	failed, landed := progress.outcomes[0], progress.outcomes[1]
	assert.Equal(t, "main", failed.Branch)
	require.ErrorContains(t, failed.Err, "add boom")
	assert.Equal(t, "develop", landed.Branch)
	require.NoError(t, landed.Err)
	created := map[string]string{}
	for _, w := range ws.Created {
		created[w.ID] = w.Branch
	}
	assert.Equal(t, "develop", created[landed.ID], "a landed branch names the workspace it created")
	assert.NotContains(t, created, failed.ID)
}

func TestImportRepo_AProtectedNameTheRepoLacksIsSkipped(t *testing.T) {
	projects, _, ws, git, prov, uc := newImport(t)
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"main", "develop", "master"}
	git.MissingLocalBranches = map[string]bool{"develop": true, "master": true}
	git.RemoteTrackingBranches = map[string]bool{"master": false}
	progress := &recordedProgress{}

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ID: "r-1", ProjectID: "proj-1", Path: "/root/repoA"}, progress)
	require.NoError(t, err)

	require.Len(t, progress.outcomes, 1)
	assert.Equal(t, "main", progress.outcomes[0].Branch)
	require.NoError(t, progress.outcomes[0].Err)
	for _, w := range ws.Created {
		assert.NotContains(t, []string{"develop", "master"}, w.Branch)
	}
}

func TestImportRepo_AProtectedBranchOnlyOriginHasIsProvisioned(t *testing.T) {
	projects, _, _, git, prov, uc := newImport(t)
	require.NoError(t, projects.Save(context.Background(), domain.Project{ID: "proj-1", Path: "/root"}))
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/root/repoA", Branch: "feature", Head: "h1"}}
	prov.Protected = []string{"develop"}
	git.MissingLocalBranches = map[string]bool{"develop": true}
	git.RemoteTrackingBranches = map[string]bool{"develop": true}
	progress := &recordedProgress{}

	_, err := uc.ImportRepo(context.Background(), project.RepoImport{ID: "r-1", ProjectID: "proj-1", Path: "/root/repoA"}, progress)
	require.NoError(t, err)
	require.Len(t, progress.outcomes, 1)
	assert.Equal(t, "develop", progress.outcomes[0].Branch)
}
