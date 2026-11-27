package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type stubFSEngine struct {
	enginefs.Engine
	watcher *enginefs.Watcher
}

func (s stubFSEngine) NewWatcher(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ enginefs.GitStatusProvider,
	_ enginefs.Dispatcher,
) *enginefs.Watcher {
	return s.watcher
}

type stubWorkspaceRepo struct {
	workspacerepo.Workspace
	ws  domain.Workspace
	err error
}

func (s stubWorkspaceRepo) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return s.ws, s.err
}

func TestNewWatcherFactory_ResolvesWorkspaceAndBuildsWatcher(t *testing.T) {
	repo := stubWorkspaceRepo{ws: domain.Workspace{ID: "w1", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned}}
	factory := newWatcherFactory(
		stubFSEngine{watcher: &enginefs.Watcher{}},
		&gitStatusProvider{engine: stubGitEngine{}},
		repo,
		stubGitEngine{},
		newWatcherDispatcher(newRecordingSignals(), repo, time.Now),
	)

	w, err := factory(context.Background(), "w1")
	require.NoError(t, err)
	assert.NotNil(t, w)
}

func TestNewWatcherFactory_GetErrorPropagates(t *testing.T) {
	repo := stubWorkspaceRepo{err: errors.New("boom")}
	factory := newWatcherFactory(
		stubFSEngine{},
		&gitStatusProvider{engine: stubGitEngine{}},
		repo,
		stubGitEngine{},
		newWatcherDispatcher(newRecordingSignals(), repo, time.Now),
	)

	w, err := factory(context.Background(), "w1")
	require.Error(t, err)
	assert.Nil(t, w)
}

func TestNewWatcher_BuildsTheProductionWatcher(t *testing.T) {
	repo := stubWorkspaceRepo{ws: domain.Workspace{ID: "w1", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned}}
	m := NewWatcher(
		context.Background(),
		newRecordingSignals(),
		repo,
		enginegit.Engine(stubGitEngine{}),
		stubFSEngine{watcher: &enginefs.Watcher{}},
		time.Now,
	)

	require.NotNil(t, m)
	require.NotPanics(t, m.Close)
}

type revParseStubEngine struct {
	enginegit.Engine
	err error
}

func (s revParseStubEngine) RevParse(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "deadbeef", s.err
}

func TestWatcherDiffBase_NoParentNoBranch_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{ForkPointSha: "fork-sha"}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{}, ws)
	assert.Equal(t, "fork-sha", got)
}

func TestWatcherDiffBase_OwnBranch_ResolvesViaRevParse(t *testing.T) {
	ws := domain.Workspace{Branch: "main", WorktreePath: "/repo", ForkPointSha: "fork-sha", Provisioning: domain.WorkspaceProvisioned}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{}, ws)
	assert.Equal(t, "main", got)
}

func TestWatcherDiffBase_OwnBranchUnresolvable_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{Branch: "deleted-branch", WorktreePath: "/repo", ForkPointSha: "fork-sha", Provisioning: domain.WorkspaceProvisioned}
	got := watcherDiffBase(context.Background(), stubWorkspaceRepo{}, revParseStubEngine{err: errors.New("unknown revision")}, ws)
	assert.Equal(t, "fork-sha", got)
}

func TestWatcherDiffBase_ChildWorkspace_UsesParentBranch(t *testing.T) {
	ws := domain.Workspace{ParentID: "parent-1", WorktreePath: "/repo", ForkPointSha: "fork-sha", Provisioning: domain.WorkspaceProvisioned}
	repo := stubWorkspaceRepo{ws: domain.Workspace{Branch: "develop"}}

	got := watcherDiffBase(context.Background(), repo, revParseStubEngine{}, ws)

	assert.Equal(t, "develop", got)
}

func TestWatcherDiffBase_ParentLookupFails_FallsBackToForkPoint(t *testing.T) {
	ws := domain.Workspace{ParentID: "gone", WorktreePath: "/repo", ForkPointSha: "fork-sha", Provisioning: domain.WorkspaceProvisioned}
	repo := stubWorkspaceRepo{err: errors.New("no such workspace")}

	got := watcherDiffBase(context.Background(), repo, revParseStubEngine{}, ws)

	assert.Equal(t, "fork-sha", got)
}
