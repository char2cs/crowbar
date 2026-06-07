package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
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
	repo := stubWorkspaceRepo{ws: domain.Workspace{ID: "w1", WorktreePath: "/repo"}}
	factory := newWatcherFactory(
		stubFSEngine{watcher: &enginefs.Watcher{}},
		&gitStatusProvider{engine: stubGitEngine{}},
		repo,
		newWatcherDispatcher(hub.NewHub(), repo, time.Now),
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
		newWatcherDispatcher(hub.NewHub(), repo, time.Now),
	)

	w, err := factory(context.Background(), "w1")
	require.Error(t, err)
	assert.Nil(t, w)
}

func TestNewWatcherManager_BuildsProductionManager(t *testing.T) {
	repo := stubWorkspaceRepo{ws: domain.Workspace{ID: "w1", WorktreePath: "/repo"}}
	m := NewWatcherManager(
		context.Background(),
		hub.NewHub(),
		repo,
		enginegit.Engine(stubGitEngine{}),
		stubFSEngine{watcher: &enginefs.Watcher{}},
		time.Now,
	)

	require.NotNil(t, m)
	require.NotPanics(t, m.StopAll)
}
