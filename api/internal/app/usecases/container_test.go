package usecases_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"

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
		newTestAsynx[domain.Chat](t, adapters.AgentChatES()),
		newTestAsynx[domain.ChatActivity](t, adapters.AgentActivityES()),
		newTestAsynx[agents.Runner](t, adapters.AgentRunnerES()),
		nil, // git conflict-checker not exercised by this test
		nil, // terminateSession not exercised by this test
		noChatWatch,
		noRunnerWatch,
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
	// Required since repo import became chat-first: adopting a repo now MINTS a
	// chat, and minting one resolves the default permission level off this
	// store. Left nil, the import panics rather than failing — which is what a
	// container fixture that lies about its wiring buys you.
	permissionDefaults, err := storesqlite.NewFromDB[domain.AgentPermissionDefault, string](globalView)
	require.NoError(t, err)
	terminalSessions, err := storesqlite.NewFromDB[domain.TerminalSession, string](globalView)
	require.NoError(t, err)

	gormStores := usecases.GORMStores{
		Projects:                 projects,
		Repositories:             repoStore,
		TerminalProfiles:         profiles,
		TerminalSessions:         terminalSessions,
		AgentProviderPreferences: providerPrefs,
		AgentPermissionDefault:   permissionDefaults,
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
	assert.NotNil(t, c.BranchReview)
	assert.NotNil(t, c.AgentChat)
	assert.NotNil(t, c.AgentTurn)
	assert.NotNil(t, c.AgentRunner)
	assert.NotNil(t, c.AgentAnswer)
	assert.NotNil(t, c.AgentProvider)
}

// TestContainer_ProductionMCPSurfaceAdvertisesEveryTool is the wiring guard as a
// RUNNING DAEMON sees it, and the one below is not a substitute for it.
//
// That one rebuilds the Deps and fills Chats, ChatLogs and Lineage by hand. In
// production nothing does: agentusecase.New binds the usecase to itself for all
// three, because they are its own methods and no caller can supply them. So a
// mistake in that self-wiring would leave the other test perfectly green while the
// daemon quietly served a shorter tool list — an agent with fewer tools than it
// should have, which nothing logs.
//
// This one asks the surface itself, over the same JSON-RPC dispatch a vendor CLI
// uses. tools/list needs no authenticated caller (authentication is per CALL), so
// the token is arbitrary and the answer is exactly what the daemon advertises.
func TestContainer_ProductionMCPSurfaceAdvertisesEveryTool(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	out, send, err := c.AgentProvider.DispatchMCP(context.Background(), "RUN", "any-token",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	require.True(t, send, "tools/list must be answered, not swallowed")

	var reply struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out, &reply))

	names := make([]string, 0, len(reply.Result.Tools))
	for _, tool := range reply.Result.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"set_chat_title",
		"set_branch_name",
		"list_review_threads",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "the tool surface a running daemon serves is incomplete — a port New is "+
		"responsible for self-wiring is nil, so the tool that needs it was withdrawn")
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
// Chats, ChatLogs and Lineage are filled in the way production fills them:
// agentusecase.New binds the usecase to itself for all three (see its doc
// comment), so c.AgentChat is the exact value the running daemon's Deps carries
// for each port.
func TestContainer_AgentToolDepsWireEveryToolGroup(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	minter, err := agentusecase.NewTokenMinter()
	require.NoError(t, err)
	deps, err := usecases.NewAgentToolDepsForTest(minter, repos, c.BranchReview, noopThreadBroadcast, c.Workspace)
	require.NoError(t, err)
	deps.Chats = c.AgentChat
	deps.ChatLogs = c.AgentChat
	deps.Lineage = c.AgentChat

	names := []string{}
	for _, tool := range agentusecase.NewToolSet(deps, "RUN", minter.Mint("RUN")).Tools() {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"set_chat_title",
		"set_branch_name",
		"list_review_threads",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "the production agent tool surface is incomplete — a port is unwired in newAgentToolDeps")
}

// TestContainer_AgentToolMetricsAreReadableFromTheContainer closes the loop the
// counters were missing: they were recorded into an instance buried inside
// agentusecase.ToolDeps, which nothing outside the tool surface could reach, so the
// number instrumentation exists to produce was unobtainable in a running daemon.
//
// The stimulus goes through DispatchMCP — the daemon's only entry point to the
// tool surface — rather than through a Metrics the test made itself, because the
// whole failure mode being guarded is an accessor wired to a DIFFERENT instance
// than production records through. A forged token is fine as the stimulus: a
// rejected call is counted too, and it is the datum this counter most exists for.
func TestContainer_AgentToolMetricsAreReadableFromTheContainer(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	require.Empty(t, c.AgentToolMetrics(), "a daemon that has served no tool call has nothing to report")

	_, _, err = c.AgentProvider.DispatchMCP(context.Background(), "RUN", "forged-token", []byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"set_chat_title","arguments":{"title":"x"}}}`,
	))
	require.NoError(t, err, "a rejected tool call is an RPC-level error, not a dispatch failure")

	require.Equal(t,
		map[string]agentusecase.ToolStat{"set_chat_title": {Calls: 1, Failures: 1}},
		c.AgentToolMetrics())
}

// A missing port is a failed start, not a silently narrowed tool list.
func TestContainer_AgentToolDeps_RefusesAPartialSurface(t *testing.T) {
	repos, _, _ := newContainerDeps(t)
	minter, err := agentusecase.NewTokenMinter()
	require.NoError(t, err)
	review := stubReviewReaderForContainer{}

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, nil, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no review reader")

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, review, nil, stubBranchRenamer{})
	require.Error(t, err, "no thread broadcaster")

	_, err = usecases.NewAgentToolDepsForTest(nil, repos, review, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no token minter")

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, review, noopThreadBroadcast, nil)
	require.Error(t, err, "no workspace usecase")

	bare := &repositories.Container{}
	_, err = usecases.NewAgentToolDepsForTest(minter, bare, review, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no repository stores")
}

// noopThreadBroadcast stands in for the app layer's hub adapter. These tests build
// the usecases container without the api layer, and what the fan-out DOES is proved
// in the agenttools package; here it only has to be non-nil so the wiring is complete.
func noopThreadBroadcast(_ domain.ReviewThread, _, _ string) {}

// stubBranchRenamer stands in for the workspace usecase in the refusal test,
// where every other port is deliberately nil in turn and this one only has to
// be non-nil so the case under test is the one that fires.
type stubBranchRenamer struct{}

func (stubBranchRenamer) RenameBranch(
	_ context.Context,
	_ string,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

type stubReviewReaderForContainer struct{}

func (stubReviewReaderForContainer) GetScope(
	_ context.Context,
	_ domain.Workspace,
) (gitdomain.ReviewScope, error) {
	return gitdomain.ReviewScope{}, nil
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

// TestWorktreeChildCreator_ForcesARealWorktree_EvenFromAWorkspacelessForkParent
// pins the Promote fix: workspace.CreateChild's own taxonomy default rule
// (model spec §4.1) inherits OwnWorktree from the PARENT, so a fork parent
// that is itself a workspace-less bubble (WorktreePath == "") would otherwise
// default a promotion into ANOTHER bubble — a chat with neither a worktree nor
// a branch name, silently. worktreeChildCreator forces OwnWorktree true
// always, which this proves against the REAL workspace.Usecase (real git, real
// resolveInherited, real branch generator), not the fake usecases/chat wires
// its own fixture with.
func TestWorktreeChildCreator_ForcesARealWorktree_EvenFromAWorkspacelessForkParent(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	require.NoError(t, gormStores.Repositories.Save(context.Background(), domain.Repository{
		ID:   "r1",
		Name: "repo",
		Path: repoDir,
	}))
	parent, err := repos.Workspace.Create(context.Background(), workspace.CreateInput{
		ID:        "parent-ws",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "main",
		// WorktreePath left empty on purpose: this is the workspace-less
		// "bubble" fork parent the reviewer's finding is about.
	}, time.Now())
	require.NoError(t, err)
	require.Empty(t, parent.WorktreePath, "precondition: the fork parent owns no worktree of its own")

	creator := usecases.NewWorktreeChildCreatorForTest(c.Workspace)

	child, err := creator.CreateChildWorkspace(context.Background(), parent.ID)

	require.NoError(t, err)
	assert.NotEmpty(t, child.WorktreePath,
		"promotion must always produce a real worktree, even forked from a workspace-less parent")
	assert.NotEmpty(t, child.Branch,
		"promotion must always produce a generated branch name")
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

// noChatWatch / noRunnerWatch are the agent announcement seams for tests that assert
// nothing about WS frames. They are non-nil on purpose: agentrunner's store REFUSES a
// nil watch at construction (a store that silently drops every frame is worse than one
// that fails to build), so `nil` here would break every container in this file.
func noChatWatch(_ agentchat.ChatEvent)       {}
func noRunnerWatch(_ agentrunner.RunnerEvent) {}
