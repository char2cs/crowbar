package repositories_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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

func newDB(
	t *testing.T,
) *gormdb.DB {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	return db
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

func (h *captureHub) BroadcastChat(
	_ hub.ChatStatusEvent,
) {
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.workspaces)
}

func (h *captureHub) lastAgentRunning(
	wsID string,
) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.workspaces) - 1; i >= 0; i-- {
		if h.workspaces[i].ID == wsID {
			return h.workspaces[i].AgentRunning, true
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
		newDB(t),
		h,
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
	)
	require.NoError(t, err)
	return c
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := newContainer(t, hub.NewHub())
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.Chat)
	assert.NotNil(t, c.AgentRun)
	assert.NotNil(t, c.ReviewThread)
}

func TestContainer_New_ClosedDBReturnsError(t *testing.T) {
	db := newDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repositories.New(
		db,
		hub.NewHub(),
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		ax[domain.AgentRun](t),
		ax[domain.ReviewThread](t),
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

func TestContainer_RegisterHubProjections_NoError(t *testing.T) {
	axRun := ax[domain.AgentRun](t)
	c, err := repositories.New(
		newDB(t),
		hub.NewHub(),
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		axRun,
		ax[domain.ReviewThread](t),
	)
	require.NoError(t, err)
	assert.NoError(t, c.RegisterHubProjections(axRun))
}

func TestContainer_RegisterHubProjections_RefreshesWorkspaceOnRun(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	axRun := ax[domain.AgentRun](t)
	c, err := repositories.New(
		newDB(t),
		h,
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		axRun,
		ax[domain.ReviewThread](t),
	)
	require.NoError(t, err)
	require.NoError(t, c.RegisterHubProjections(axRun))

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
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
	before := h.count()

	_, err = c.Chat.Create(ctx, "c1", "w1", "t", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.MarkRunning(ctx, "a1")
	require.NoError(t, err)

	require.Eventually(t, func() bool { return h.count() > before }, time.Second, 5*time.Millisecond)
}

func TestContainer_BroadcastWorkspace_CarriesAgentRunningOverlay(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	axRun := ax[domain.AgentRun](t)
	c, err := repositories.New(
		newDB(t),
		h,
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		axRun,
		ax[domain.ReviewThread](t),
	)
	require.NoError(t, err)
	require.NoError(t, c.RegisterHubProjections(axRun))

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, ok := h.lastAgentRunning("w1")
		return ok && !running
	}, time.Second, 5*time.Millisecond)

	_, err = c.Chat.Create(ctx, "c1", "w1", "t", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, ok := h.lastAgentRunning("w1")
		return ok && running
	}, time.Second, 5*time.Millisecond)

	_, err = c.AgentRun.Complete(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, ok := h.lastAgentRunning("w1")
		return ok && !running
	}, time.Second, 5*time.Millisecond)
}

type listRunningErrRepo struct {
	agentrun.AgentRun
}

func (listRunningErrRepo) ListRunning(
	_ context.Context,
) ([]domain.AgentRun, error) {
	return nil, errFake
}

func TestContainer_BroadcastWorkspace_ListRunningErrorOverlayFalse(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	c := newContainer(t, h)
	c.AgentRun = listRunningErrRepo{}

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, ok := h.lastAgentRunning("w1")
		return ok && !running
	}, time.Second, 5*time.Millisecond)
}

func TestContainer_RecoverOrphans_FlipsRunningToError(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, hub.NewHub())

	_, err := c.Chat.Create(ctx, "c1", "w1", "t", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Chat.SetAgentRunning(ctx, "c1")
	require.NoError(t, err)
	_, err = c.AgentRun.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := c.AgentRun.ListRunning(ctx)
		return listErr == nil && len(running) == 1
	}, time.Second, 5*time.Millisecond)

	c.RecoverOrphans(ctx)

	run, err := c.AgentRun.Get(ctx, "a1")
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusError, run.Status)
	ch, err := c.Chat.Get(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, domain.ChatStatusIdle, ch.Status)
}

func TestContainer_RecoverOrphans_EmptyIsNoOp(t *testing.T) {
	c := newContainer(t, hub.NewHub())
	assert.NotPanics(t, func() { c.RecoverOrphans(context.Background()) })
}

func TestContainer_RefreshWorkspace_MissingWorkspaceIsNoOp(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	axRun := ax[domain.AgentRun](t)
	c, err := repositories.New(
		newDB(t),
		h,
		ax[domain.Workspace](t),
		ax[domain.Chat](t),
		axRun,
		ax[domain.ReviewThread](t),
	)
	require.NoError(t, err)
	require.NoError(t, c.RegisterHubProjections(axRun))

	_, err = c.AgentRun.Create(ctx, "a1", "wmissing", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.MarkRunning(ctx, "a1")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		running, listErr := c.AgentRun.ListRunning(ctx)
		return listErr == nil && len(running) == 1
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, h.count())
}
