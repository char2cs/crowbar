package usecases_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func newTestAsynx[T any](
	t *testing.T,
	es asynxModels.Store,
) asynx.Asynx[T] {
	t.Helper()
	ax, err := asynx.New[T]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func newContainerDeps(
	t *testing.T,
) (
	*repositories.Container,
	usecases.GORMStores,
	*engine.Container,
) {
	t.Helper()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	repos, err := repositories.New(
		context.Background(),
		adapters,
		hub.NewHub(),
		newTestAsynx[domain.ReviewThread](t, adapters.ReviewThreadES()),
		newTestAsynx[domain.Workspace](t, adapters.WorkspaceES()),
		newTestAsynx[domain.AgentChat](t, adapters.AgentChatES()),
		newTestAsynx[domain.AgentRunner](t, adapters.AgentRunnerES()),
		nil, // git conflict-checker not exercised by this test
		nil, // terminateSession not exercised by this test
	)
	require.NoError(t, err)

	globalView := adapters.GlobalView()
	projects, err := storesqlite.NewFromDB[domain.Project, string](globalView)
	require.NoError(t, err)
	repoStore, err := storesqlite.NewFromDB[domain.Repository, string](globalView)
	require.NoError(t, err)
	profiles, err := storesqlite.NewFromDB[domain.TerminalProfile, string](globalView)
	require.NoError(t, err)

	gormStores := usecases.GORMStores{
		Projects:         projects,
		Repositories:     repoStore,
		TerminalProfiles: profiles,
	}

	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	return repos, gormStores, eng
}

func TestContainer_New_BuildsEveryUsecase(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)

	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil })
	require.NoError(t, err)

	assert.NotNil(t, c.Project)
	assert.NotNil(t, c.ProjectImport)
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.File)
	assert.NotNil(t, c.Git)
	assert.NotNil(t, c.Terminal)
	assert.NotNil(t, c.ProviderSync)
	assert.NotNil(t, c.Worktree)
	assert.NotNil(t, c.BranchReview)
	assert.NotNil(t, c.Agent)
}

func TestContainer_FileTree_DelegatesToRealFsEngine(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil })
	require.NoError(t, err)

	dir := t.TempDir()
	now := time.Unix(1, 0).UTC()
	_, err = repos.Workspace.Create(
		t.Context(),
		workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: dir},
		now,
	)
	require.NoError(t, err)

	nodes, err := c.File.Tree(t.Context(), "w1", "", containerStatusStub{})
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

func TestContainer_Import_ResolvesDefaultBranchViaRealGit(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil })
	require.NoError(t, err)

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	got, err := c.ProjectImport.Import(t.Context(), "p", root)
	require.NoError(t, err)
	assert.Equal(t, "p", got.Name)
}

func runGit(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type containerStatusStub struct{}

func (containerStatusStub) GitStatus(
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}
