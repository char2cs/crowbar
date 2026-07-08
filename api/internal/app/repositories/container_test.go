package repositories_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
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

// wsAx builds the singleton workspace asynx over the adapter's per-type event
// store — the same handle workspace.New/store.New project onto.
func wsAx(
	t *testing.T,
	ad *adapter.Container,
) asynx.Asynx[domain.Workspace] {
	t.Helper()
	a, err := asynx.New[domain.Workspace]().
		WithEventStore(ad.WorkspaceES()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	return a
}

type captureHub struct {
	hub.WebSocketHub
	mu         sync.Mutex
	workspaces []dto.WorkspaceDTO
}

func (h *captureHub) BroadcastWorkspace(
	ws dto.WorkspaceDTO,
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

func (h *captureHub) last(
	wsID string,
) (dto.WorkspaceDTO, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.workspaces) - 1; i >= 0; i-- {
		if h.workspaces[i].ID == wsID {
			return h.workspaces[i], true
		}
	}
	return dto.WorkspaceDTO{}, false
}

func newContainer(
	t *testing.T,
	h hub.WebSocketHub,
) *repositories.Container {
	t.Helper()
	ad := newAdapter(t)
	c, err := repositories.New(
		context.Background(),
		ad,
		h,
		ax[domain.ReviewThread](t),
		wsAx(t, ad),
		nil,
	)
	require.NoError(t, err)
	return c
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := newContainer(t, hub.NewHub())
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.ReviewThread)
}

func TestContainer_New_NilWorkspaceAxReturnsError(t *testing.T) {
	_, err := repositories.New(
		context.Background(),
		newAdapter(t),
		hub.NewHub(),
		ax[domain.ReviewThread](t),
		nil, // nil axWorkspace → workspace.New rejects
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

// TestBroadcastWorkspace_WorkingFalse pins the idle baseline of the working
// overlay: with no background mutation in flight, every broadcast carries
// Working=false.
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

// TestContainer_ListWorkspaces_NoWorkingOverlay asserts the snapshot source
// returns workspace rows with the working overlay false while no background
// mutation is in flight.
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

// TestContainer_BeginEndWork_BroadcastsWorkingOverlay pins the real working
// overlay (00 §4 async mutations): BeginWork immediately re-broadcasts the row
// with Working=true, event-driven frames emitted while the mutation runs carry
// Working=true, and EndWork re-broadcasts with Working=false so the client
// spinner always resolves.
func TestContainer_BeginEndWork_BroadcastsWorkingOverlay(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	c := newContainer(t, h)

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, ok := h.last("w1")
		return ok
	}, time.Second, 5*time.Millisecond)

	c.BeginWork(ctx, "w1")
	working, ok := h.lastWorking("w1")
	require.True(t, ok)
	assert.True(t, working, "BeginWork must broadcast Working=true")
	assert.True(t, c.IsWorking("w1"))

	c.EndWork(ctx, "w1")
	working, ok = h.lastWorking("w1")
	require.True(t, ok)
	assert.False(t, working, "EndWork must broadcast Working=false")
	assert.False(t, c.IsWorking("w1"))
}

// TestContainer_BeginWork_NestsPerWorkspace asserts overlapping background
// mutations on the same workspace stay Working until the LAST one ends, and
// that blank ids (a create that has no entity yet) are ignored.
func TestContainer_BeginWork_NestsPerWorkspace(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	c.BeginWork(ctx, "w1")
	c.BeginWork(ctx, "w1")
	c.EndWork(ctx, "w1")
	assert.True(t, c.IsWorking("w1"))
	c.EndWork(ctx, "w1")
	assert.False(t, c.IsWorking("w1"))

	c.EndWork(ctx, "w1")
	assert.False(t, c.IsWorking("w1"), "unbalanced EndWork must not underflow")

	c.BeginWork(ctx, "")
	assert.False(t, c.IsWorking(""), "blank ids are ignored")
}

// TestContainer_ListWorkspaces_WorkingOverlay asserts the snapshot source
// carries the live working overlay, so a client subscribing mid-mutation sees
// the spinner state without waiting for the next broadcast.
func TestContainer_ListWorkspaces_WorkingOverlay(t *testing.T) {
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

	c.BeginWork(ctx, "w1")
	rows, err := c.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Working)

	c.EndWork(ctx, "w1")
	rows, err = c.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.False(t, rows[0].Working)
}

// TestBroadcastWorkspace_ResolvesMergeEligibility pins that the broadcast DTO
// carries the merge-eligibility overlay resolved from the row's repo siblings
// (spec §10): a child whose parent is a same-repo non-locked sibling is eligible
// with the parent's branch, while the parent itself is not eligible.
func TestBroadcastWorkspace_ResolvesMergeEligibility(t *testing.T) {
	ctx := context.Background()
	h := &captureHub{}
	c := newContainer(t, h)

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "parent", RepoID: "r1", ProjectID: "p1", Branch: "main",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "child", RepoID: "r1", ProjectID: "p1", Branch: "feat", ParentID: "parent",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, ok := h.last("child")
		return ok && d.CanMergeLocally
	}, time.Second, 5*time.Millisecond)

	child, ok := h.last("child")
	require.True(t, ok)
	assert.True(t, child.CanMergeLocally)
	assert.Equal(t, "main", child.ParentBranch)

	parent, ok := h.last("parent")
	require.True(t, ok)
	assert.False(t, parent.CanMergeLocally)
	assert.Equal(t, "", parent.ParentBranch)
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

// TestContainer_WireCallbacks_DeleteCascade pins the cross-aggregate delete
// cascade wireCallbacks wires (Task 14, spec §3.6): deleting a workspace fires the
// pure Delete command, and the async delete reactor — gated on the persisted
// "deleted" tombstone — forgets every review thread anchored to the workspace
// (their rows vanish), rm -rf's the worktree, and Forgets the workspace aggregate
// (dropping its read-model row). Before wireCallbacks was wired, Delete only
// tombstoned the row and nothing purged, so this cascade never converged.
func TestContainer_WireCallbacks_DeleteCascade(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	c, err := repositories.New(ctx, ad, &captureHub{}, ax[domain.ReviewThread](t), wsAx(t, ad), nil)
	require.NoError(t, err)

	// A real MANAGED worktree UNDER the crowbar home: the delete reactor's rm is
	// guarded to the home (an adopted checkout outside the home is never touched, so
	// a delete can never destroy a user's real repository), so the reaped worktree
	// must live under <home>/projects/... like a real crowbar-managed one.
	worktree := filepath.Join(ad.CrowbarHome(), "projects", "p1", "managed", "b")
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "f"), []byte("x"), 0o600))

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", WorktreePath: worktree,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	// A review thread anchored to the workspace: the cascade must forget it.
	_, err = c.ReviewThread.Open(ctx, reviewthread.OpenInput{
		ID: "t1", WsID: "w1", FilePath: "a.go", MessageID: "m1", Author: "u", Body: "hi",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)

	// The thread's read-model row must be present before we delete, so the cascade
	// has a row to forget.
	require.Eventually(t, func() bool {
		threads, e := c.ReviewThread.ListByWorkspace(ctx, "w1")
		return e == nil && len(threads) == 1
	}, time.Second, 5*time.Millisecond)

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))

	// The async delete reactor cascades: review threads forgotten (rows gone),
	// worktree removed, workspace aggregate Forgotten (read-model row gone).
	require.Eventually(t, func() bool {
		threads, e := c.ReviewThread.ListByWorkspace(ctx, "w1")
		if e != nil || len(threads) != 0 {
			return false
		}
		rows, e2 := c.Workspace.List(ctx)
		if e2 != nil || len(rows) != 0 {
			return false
		}
		_, statErr := os.Stat(worktree)
		return os.IsNotExist(statErr)
	}, 3*time.Second, 10*time.Millisecond)
}

// TestContainer_WireCallbacks_DeleteNeverRmsAdoptedCheckout pins the delete-cascade
// DATA-LOSS guard: an adopted home/main workspace's WorktreePath is the user's REAL
// checkout (repo.Path/project.Path), which lives OUTSIDE the crowbar home. Deleting
// it must reap the record while the async reactor's rm — guarded to the crowbar home
// — leaves the on-disk checkout untouched. An unguarded os.RemoveAll here would
// os.RemoveAll the user's repository (the regression this guard prevents).
func TestContainer_WireCallbacks_DeleteNeverRmsAdoptedCheckout(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	c, err := repositories.New(ctx, ad, &captureHub{}, ax[domain.ReviewThread](t), wsAx(t, ad), nil)
	require.NoError(t, err)

	// The user's real checkout, OUTSIDE the crowbar home (an adopted worktree).
	adopted := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(adopted, "README.md"), []byte("real"), 0o600))

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "home1", RepoID: "r1", ProjectID: "p1", Branch: "main",
		WorktreePath: adopted, Kind: domain.WorkspaceKindHome,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	require.NoError(t, c.Workspace.Delete(ctx, "home1"))

	// The record is reaped (aggregate Forgotten, read-model row gone) ...
	require.Eventually(t, func() bool {
		rows, e := c.Workspace.List(ctx)
		return e == nil && len(rows) == 0
	}, 3*time.Second, 10*time.Millisecond, "the deleted workspace record must be reaped")

	// ... but the user's real checkout on disk must survive untouched.
	_, statErr := os.Stat(filepath.Join(adopted, "README.md"))
	require.NoError(t, statErr,
		"an adopted checkout outside the crowbar home must never be rm'd by a workspace delete")
}
