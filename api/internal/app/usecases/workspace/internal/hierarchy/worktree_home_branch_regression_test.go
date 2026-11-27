package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func homeBranchHarness(
	t *testing.T,
	repoPath string,
) (hierarchy.Usecase, workspace.Workspace, func()) {
	t.Helper()

	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	workspaces, quiesce := newWorkspaceRepo(t, adapters)
	repos, err := storesqlite.NewFromDB[domain.Repository, string](adapters.GlobalView())
	require.NoError(t, err)
	require.NoError(t, repos.Save(context.Background(), domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Name:          "repo",
		Path:          repoPath,
		DefaultBranch: "main",
	}))

	crowbarHome := t.TempDir()
	uc := hierarchy.New(
		workspaces,
		enginegit.New(),
		&stubProvider{},
		repos,
		func() time.Time { return time.Unix(1000, 0).UTC() },
		func() (string, error) { return crowbarHome, nil },
	)
	withOwningChats(uc)
	return uc, workspaces, quiesce
}

func TestRegression_CreateChildDetachingTheHome_ClearsTheHomeRowBranch(t *testing.T) {
	repoPath, _ := setupImportRepo(t)
	uc, workspaces, quiesce := homeBranchHarness(t, repoPath)
	ctx := context.Background()

	home, err := workspaces.Create(ctx, workspace.CreateInput{
		ID:           "home",
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "main",
		WorktreePath: repoPath,
		IsDefault:    true,
		Provisioning: domain.WorkspaceShared,
	}, time.Unix(1000, 0).UTC())
	require.NoError(t, err)

	child, err := uc.CreateChild(ctx, hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     repoPath,
		Branch:       "main",
		ParentBranch: "main",
	})
	require.NoError(t, err, "the create detaches the home to free the branch and retries")
	require.NotEmpty(t, child.WorktreePath, "the branch landed in a real managed worktree")

	quiesce()
	got, err := workspaces.Get(ctx, home.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Branch,
		"the home row must stop claiming a branch git detached it from")

	all, err := workspaces.List(ctx)
	require.NoError(t, err)
	onMain := 0
	for _, w := range all {
		if w.Branch == "main" {
			onMain++
		}
	}
	assert.Equal(t, 1, onMain, "exactly one workspace claims the branch")
}

func TestRegression_CreateChildOffADetachedHome_ForksFromItsCommit(t *testing.T) {
	repoPath, _ := setupImportRepo(t)
	gitRun(t, repoPath, "checkout", "--detach")
	head := revParse(t, repoPath, "HEAD")
	uc, workspaces, _ := homeBranchHarness(t, repoPath)
	ctx := context.Background()

	_, err := workspaces.Create(ctx, workspace.CreateInput{
		ID:           "home",
		RepoID:       "r1",
		ProjectID:    "p1",
		WorktreePath: repoPath,
		IsDefault:    true,
		Provisioning: domain.WorkspaceShared,
	}, time.Unix(1000, 0).UTC())
	require.NoError(t, err)

	ownWorktree := true
	child, err := uc.CreateChild(ctx, hierarchy.CreateChildInput{
		ParentID:    "home",
		Branch:      "ui-branch",
		OwnWorktree: &ownWorktree,
	})
	require.NoError(t, err)
	assert.Equal(t, head, child.ForkPointSha, "the branch forks from the commit the home holds")
	assert.Equal(t, head, revParse(t, child.WorktreePath, "HEAD"))
}
