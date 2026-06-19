package repositories_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errFake = errors.New("fake error")

func ax[T any](
	t *testing.T,
) asynx.Asynx[T] {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	a, err := asynx.New[T]().WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	return a
}

func newAdapter(
	t *testing.T,
) *adapter.Container {
	t.Helper()
	c, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func wsFactory(
	es asynxModels.Store,
) (asynx.Asynx[domain.Workspace], error) {
	return asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

type captureHub struct {
	hub.WebSocketHub
	mu         sync.Mutex
	workspaces []domain.Workspace
}

func (h *captureHub) BroadcastWorkspace(
	ws domain.Workspace,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaces = append(h.workspaces, ws)
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.workspaces)
}

func (h *captureHub) lastWorking(
	wsID string,
) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.workspaces) - 1; i >= 0; i-- {
		if h.workspaces[i].ID == wsID {
			return h.workspaces[i].Working, true
		}
	}
	return false, false
}

func newContainer(
	t *testing.T,
	h hub.WebSocketHub,
) *repositories.Container {
	t.Helper()
	c, err := repositories.New(
		newAdapter(t),
		h,
		ax[domain.Chat](t),
		ax[domain.ReviewThread](t),
		wsFactory,
	)
	require.NoError(t, err)
	return c
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := newContainer(t, hub.NewHub())
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.Chat)
	assert.NotNil(t, c.ReviewThread)
}

func TestContainer_New_NilFactoryReturnsError(t *testing.T) {
	_, err := repositories.New(
		newAdapter(t),
		hub.NewHub(),
		ax[domain.Chat](t),
		ax[domain.ReviewThread](t),
		nil,
	)
	assert.Error(t, err)
}

func TestContainer_CreateWorkspace_ProjectsAndBroadcasts(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	c := newContainer(t, h)

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		list, listErr := c.Workspace.List(ctx)
		return listErr == nil && len(list) == 1
	}, time.Second, 5*time.Millisecond)

	list, err := c.Workspace.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, "w1", list[0].ID)
	assert.GreaterOrEqual(t, h.count(), 1)
}

// TestBroadcastWorkspace_WorkingFalse pins the post-removal overlay behaviour:
// with the agent-run producer gone, every broadcast carries Working=false
// (00 §5).
func TestBroadcastWorkspace_WorkingFalse(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	c := newContainer(t, h)

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		working, ok := h.lastWorking("w1")
		return ok && !working
	}, time.Second, 5*time.Millisecond)
}

// TestContainer_ListWorkspaces_NoWorkingOverlay asserts the snapshot
// source returns workspace rows with the working overlay always false (00 §5).
func TestContainer_ListWorkspaces_NoWorkingOverlay(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		list, listErr := c.Workspace.List(ctx)
		return listErr == nil && len(list) == 1
	}, time.Second, 5*time.Millisecond)

	rows, err := c.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].Working)
}

type listErrWorkspaceRepo struct {
	workspace.Workspace
}

func (listErrWorkspaceRepo) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, errFake
}

func TestContainer_ListWorkspaces_ListErrorPropagates(t *testing.T) {
	c := newContainer(t, &captureHub{})
	c.Workspace = listErrWorkspaceRepo{}

	rows, err := c.ListWorkspaces(context.Background())
	require.Error(t, err)
	assert.Nil(t, rows)
}
