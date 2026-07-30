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
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
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
	providerPrefs, err := storesqlite.NewFromDB[domain.AgentProviderPreference, string](globalView)
	require.NoError(t, err)

	gormStores := usecases.GORMStores{
		Projects:                 projects,
		Repositories:             repoStore,
		TerminalProfiles:         profiles,
		AgentProviderPreferences: providerPrefs,
	}

	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	return repos, gormStores, eng
}

func TestContainer_New_BuildsEveryUsecase(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)

	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
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

// TestContainer_AgentToolDepsWireEveryToolGroup is the wiring guard.
//
// agenttools registers no tool whose port is nil, so a group the container forgets
// to hand a dependency to simply does not exist — and the only symptom in a running
// daemon is an agent with fewer tools than it should have, which nothing logs and no
// unit test in agenttools can see (its own fixtures supply their own deps). This
// asserts the PRODUCTION Deps, built by the real newAgentToolDeps over the real
// repositories, advertises the complete surface by name.
//
// Chats is filled in the way production fills it: agent.New assigns the usecase to
// itself as the ChatRenamer (see its doc comment), so c.Agent is the exact value the
// running daemon's Deps carries.
func TestContainer_AgentToolDepsWireEveryToolGroup(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	deps, err := usecases.NewAgentToolDepsForTest(minter, repos, c.BranchReview, noopThreadBroadcast)
	require.NoError(t, err)
	deps.Chats = c.Agent

	names := []string{}
	for _, tool := range agenttools.NewToolSet(deps, "RUN", minter.Mint("RUN")).Tools() {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"set_chat_title",
		"list_review_threads",
		"get_review_scope",
		"post_review_comment",
	}, names, "the production agent tool surface is incomplete — a port is unwired in newAgentToolDeps")
}

// A missing port is a failed start, not a silently narrowed tool list.
func TestContainer_AgentToolDeps_RefusesAPartialSurface(t *testing.T) {
	repos, _, _ := newContainerDeps(t)
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	review := stubReviewReaderForContainer{}

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, nil, noopThreadBroadcast)
	require.Error(t, err, "no review reader")

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, review, nil)
	require.Error(t, err, "no thread broadcaster")

	_, err = usecases.NewAgentToolDepsForTest(nil, repos, review, noopThreadBroadcast)
	require.Error(t, err, "no token minter")

	bare := &repositories.Container{}
	_, err = usecases.NewAgentToolDepsForTest(minter, bare, review, noopThreadBroadcast)
	require.Error(t, err, "no repository stores")
}

// noopThreadBroadcast stands in for the app layer's hub adapter. These tests build
// the usecases container without the api layer, and what the fan-out DOES is proved
// in the agenttools package; here it only has to be non-nil so the wiring is complete.
func noopThreadBroadcast(_ domain.ReviewThread, _, _ string) {}

type stubReviewReaderForContainer struct{}

func (stubReviewReaderForContainer) GetBase(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

func (stubReviewReaderForContainer) GetFiles(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.ReviewFileSummary, error) {
	return nil, nil
}

func (stubReviewReaderForContainer) GetOutline(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.FileOutline, error) {
	return nil, nil
}

func TestContainer_FileTree_DelegatesToRealFsEngine(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
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
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
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
