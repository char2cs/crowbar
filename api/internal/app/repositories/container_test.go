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

func (h *captureHub) agentRunningHistory(
	wsID string,
) []bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []bool
	for _, ws := range h.workspaces {
		if ws.ID == wsID {
			out = append(out, ws.AgentRunning)
		}
	}
	return out
}

// TestContainer_AgentRunCompletion_OverlayClearsAfterStorePersists is the
// regression test for the stuck-spinner race: the workspace overlay used to be
// refreshed from the chat-status projector, which races the store projection's
// Save on the same asynx topic. When that projector ran first on a terminal
// event, ListRunning still reported the run as running, so the final broadcast
// carried AgentRunning=true with no corrected re-broadcast to follow (the store
// projection's broadcast was a no-op). With the overlay refresh driven from
// inside the store projection (after Save), the last completion broadcast is
// guaranteed AgentRunning=false. On the old wiring the final broadcast for w1
// could remain true, so the trailing assertion would fail.
func TestContainer_AgentRunCompletion_OverlayClearsAfterStorePersists(t *testing.T) {
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
		running, listErr := c.AgentRun.ListRunning(ctx)
		return listErr == nil && len(running) == 0
	}, time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		running, ok := h.lastAgentRunning("w1")
		return ok && !running
	}, time.Second, 5*time.Millisecond)

	hist := h.agentRunningHistory("w1")
	require.NotEmpty(t, hist)
	assert.False(t, hist[len(hist)-1], "final w1 broadcast must clear the spinner")
}

// TestAgentRunStoreBroadcast_FiresAfterSave proves the structural guarantee the
// fix relies on: the AgentRun store projection broadcasts each row only AFTER it
// has been saved to the read model. The overlay refresh is wired onto this
// broadcast, so when it recomputes ListRunning the just-applied terminal status
// is already visible. The old wiring refreshed from the chat-status projector,
// a sibling subscriber on the same topic that races this Save; on that path
// ListRunning could still report the completed run as running. This test fails
// on any wiring where the broadcast can observe a stale read model on terminal
// transitions.
func TestAgentRunStoreBroadcast_FiresAfterSave(t *testing.T) {
	ctx := context.Background()
	axRun := ax[domain.AgentRun](t)
	db := newDB(t)

	var mu sync.Mutex
	var sawRunningOnComplete bool
	var runs agentrun.AgentRun
	runs, err := agentrun.New(axRun, db, func(run domain.AgentRun) {
		if run.ID != "a1" || run.Status != domain.AgentRunStatusDone {
			return
		}
		running, listErr := runs.ListRunning(ctx)
		mu.Lock()
		defer mu.Unlock()
		if listErr == nil {
			for _, r := range running {
				if r.ID == "a1" {
					sawRunningOnComplete = true
				}
			}
		}
	})
	require.NoError(t, err)

	_, err = runs.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = runs.Complete(ctx, "a1")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		running, listErr := runs.ListRunning(ctx)
		return listErr == nil && len(running) == 0
	}, time.Second, 5*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, sawRunningOnComplete, "store broadcast must observe the terminal status it just saved")
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

type listErrWorkspaceRepo struct {
	workspace.Workspace
}

func (listErrWorkspaceRepo) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return nil, errFake
}

func TestContainer_ListWorkspacesWithOverlay_CarriesAgentRunning(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w2", RepoID: "r2", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Chat.Create(ctx, "c1", "w1", "t", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.Create(ctx, "a1", "w1", "c1", time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.AgentRun.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		running, listErr := c.AgentRun.ListRunning(ctx)
		return listErr == nil && len(running) == 1
	}, time.Second, 5*time.Millisecond)

	rows, err := c.ListWorkspacesWithOverlay(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	overlay := map[string]bool{}
	for _, row := range rows {
		overlay[row.ID] = row.AgentRunning
	}
	assert.True(t, overlay["w1"])
	assert.False(t, overlay["w2"])
}

func TestContainer_ListWorkspacesWithOverlay_ListRunningErrorOverlayFalse(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	c.AgentRun = listRunningErrRepo{}

	rows, err := c.ListWorkspacesWithOverlay(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].AgentRunning)
}

func TestContainer_ListWorkspacesWithOverlay_NilAgentRunOverlayFalse(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	c.AgentRun = nil

	rows, err := c.ListWorkspacesWithOverlay(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].AgentRunning)
}

func TestContainer_ListWorkspacesWithOverlay_ListErrorPropagates(t *testing.T) {
	c := newContainer(t, &captureHub{})
	c.Workspace = listErrWorkspaceRepo{}

	rows, err := c.ListWorkspacesWithOverlay(context.Background())
	require.Error(t, err)
	assert.Nil(t, rows)
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
