package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
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
	usecases.ProjectImportUsecase,
) {
	t.Helper()
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()
	uc := usecases.NewProjectImport(usecases.ProjectImportDeps{
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
	})
	return projects, repos, ws, git, prov, uc
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
		byBranch[w.Branch] = w.Locked
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
	uc := usecases.NewProjectImport(usecases.ProjectImportDeps{
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
		Now: time.Now,
	})

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}

func TestImport_RepoSaveError(
	t *testing.T,
) {
	_, repos, _, _, _, uc := newImport(t)
	repos.SaveErr = errors.New("repo boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}

func TestImport_WorktreeListError(
	t *testing.T,
) {
	_, _, _, git, _, uc := newImport(t)
	git.WorktreeListErr = errors.New("wt boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}

func TestImport_ProtectedBranchesError(
	t *testing.T,
) {
	_, _, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	prov.ProtectedErr = errors.New("prov boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
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

func TestImport_WorkspaceCreateError(
	t *testing.T,
) {
	_, _, ws, git, _, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	ws.CreateErr = errors.New("ws boom")

	_, err := uc.Import(context.Background(), "P", "/root")
	assert.Error(t, err)
}
