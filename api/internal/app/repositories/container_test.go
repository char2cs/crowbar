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
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
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

// agentChatAx builds the singleton agentchat asynx over the adapter's per-type
// event store — the same handle agentchat.NewEventSourced/store.New project
// onto (Task 9, additive).
func agentChatAx(
	t *testing.T,
	ad *adapter.Container,
) asynx.Asynx[domain.AgentChat] {
	t.Helper()
	a, err := asynx.New[domain.AgentChat]().
		WithEventStore(ad.AgentChatES()).
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
		agentChatAx(t, ad),
		nil,
		nil, // terminateSession not exercised by this helper's callers
	)
	require.NoError(t, err)
	return c
}

func TestContainer_New_BuildsRepos(t *testing.T) {
	c := newContainer(t, hub.NewHub())
	assert.NotNil(t, c.Workspace)
	assert.NotNil(t, c.ReviewThread)
	assert.NotNil(t, c.AgentChat)
}

func TestContainer_New_NilWorkspaceAxReturnsError(t *testing.T) {
	ad := newAdapter(t)
	_, err := repositories.New(
		context.Background(),
		ad,
		hub.NewHub(),
		ax[domain.ReviewThread](t),
		nil, // nil axWorkspace → workspace.New rejects
		agentChatAx(t, ad),
		nil,
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

	c.WaitQuiescent()

	list, err := c.Workspace.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
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

	c.WaitQuiescent()

	working, ok := h.lastWorking("w1")
	require.True(t, ok)
	assert.False(t, working)
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
	c.WaitQuiescent()

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
	c.WaitQuiescent()
	_, seen := h.last("w1")
	require.True(t, seen, "create must broadcast the workspace row")

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
	c.WaitQuiescent()

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
	// Drain the parent's store projection before creating the child. The child's
	// hub frame resolves merge eligibility by reading its repo siblings, so the
	// parent row must already be in the read model when the child event is
	// projected. Without this barrier the child's single broadcast races the
	// parent's projection across the two concurrent per-aggregate workers, and
	// CanMergeLocally would be non-deterministic. This makes it deterministic.
	c.WaitQuiescent()
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "child", RepoID: "r1", ProjectID: "p1", Branch: "feat", ParentID: "parent",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)

	c.WaitQuiescent()

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
	c, err := repositories.New(ctx, ad, &captureHub{}, ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, nil)
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
	c.WaitQuiescent()
	threads, err := c.ReviewThread.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	require.Len(t, threads, 1)

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))

	// The async delete reactor cascades: review threads forgotten (rows gone),
	// worktree removed, workspace aggregate Forgotten (read-model row gone). The
	// reactor detaches into a drainWG-tracked goroutine (its terminal Forget is a
	// SendWait that cannot run on the bus goroutine), so draining the projection
	// queues alone would not cover it. First WaitQuiescent so the delete event is
	// dispatched — the reactor has joined drainWG (onEvent's Add(1)) and the store
	// projection has written the tombstone the reactor gates on — then block on the
	// reactor's own drain WaitGroup for the cascade to finish, then WaitQuiescent
	// again to settle the follow-on Forget/DeleteThread projections. Deterministic.
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	threads, err = c.ReviewThread.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Empty(t, threads)
	rows, err := c.Workspace.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, rows)
	_, statErr := os.Stat(worktree)
	assert.True(t, os.IsNotExist(statErr))
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
	c, err := repositories.New(ctx, ad, &captureHub{}, ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, nil)
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

	// The record is reaped (aggregate Forgotten, read-model row gone) ... The reactor
	// runs the cascade in a drainWG-tracked goroutine: WaitQuiescent so the delete
	// event is dispatched (reactor joined drainWG + tombstone written), block on the
	// reactor drain for the goroutine to finish, then WaitQuiescent to settle the
	// terminal Forget projection that drops the row. Deterministic, no polling.
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	rows, err := c.Workspace.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, rows, "the deleted workspace record must be reaped")

	// ... but the user's real checkout on disk must survive untouched.
	_, statErr := os.Stat(filepath.Join(adopted, "README.md"))
	require.NoError(t, statErr,
		"an adopted checkout outside the crowbar home must never be rm'd by a workspace delete")
}

// fakeTerminateSession is a thread-safe terminateSession double: it records
// every sessionID it is asked to terminate, standing in for the injected
// terminal-engine seam repositories.Container.forgetAgentChats uses to kill a
// chat's live PTY before Forgetting it (Task 12). failFor makes terminate
// return an error for one specific session id, so a test can prove the cascade
// treats a terminate failure as best-effort (logs + continues).
type fakeTerminateSession struct {
	mu      sync.Mutex
	calls   []string
	failFor string
}

func (f *fakeTerminateSession) terminate(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sessionID)
	if sessionID == f.failFor {
		return errFake
	}
	return nil
}

func (f *fakeTerminateSession) terminated() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// createAgentChat seeds a fresh AgentChat with one active segment bound to
// wsID, mirroring the usecase's spawnSegment Create call.
func createAgentChat(
	t *testing.T,
	ctx context.Context,
	repo agentchat.EventStore,
	chatID, wsID, terminalSession string,
) {
	t.Helper()
	_, err := repo.Create(ctx, agentchat.CreateInput{
		ID:              chatID,
		WorkspaceID:     wsID,
		SegmentID:       chatID + "-seg1",
		TerminalSession: terminalSession,
		ProviderID:      "claude",
		Now:             time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
}

// TestContainer_WireCallbacks_DeleteCascade_ForgetsAgentChats pins the Task 12
// half of the delete cascade this test file's other TestContainer_WireCallbacks_*
// cases don't cover: deleting a workspace terminates its AgentChats' live PTYs
// (via the injected terminateSession seam) and Forgets the chats outright — a
// subsequent GetChat genuinely reports not found (Forget purges the read-model
// row AND the underlying event log, unlike the chat's own soft-delete Delete
// command). A chat bound to a DIFFERENT, untouched workspace must survive.
func TestContainer_WireCallbacks_DeleteCascade_ForgetsAgentChats(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	term := &fakeTerminateSession{}
	// hub.NewHub() (not &captureHub{}, which only overrides BroadcastWorkspace):
	// agentchat's hub projection fires on every event, including this test's
	// AgentChat Create/Forget, so it needs a real BroadcastAgentChat to call.
	c, err := repositories.New(ctx, ad, hub.NewHub(), ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, term.terminate)
	require.NoError(t, err)

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "b2",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	createAgentChat(t, ctx, c.AgentChat, "chat1", "w1", "term-1")
	createAgentChat(t, ctx, c.AgentChat, "chat2", "w2", "term-2")
	c.WaitQuiescent()

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))

	// Same deterministic barrier as the review-thread cascade tests above:
	// WaitQuiescent so the delete event dispatches and the reactor joins
	// drainWG, block on the reactor's own drain WaitGroup for the cascade
	// goroutine to finish, then WaitQuiescent again to settle the follow-on
	// Forget projections.
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	// chat1's PTY was terminated before it was Forgotten.
	assert.Equal(t, []string{"term-1"}, term.terminated())

	// chat1 is genuinely gone: Forget purges the read model AND the event log,
	// so GetChat cannot self-heal it back via lazy Replay.
	_, err = c.AgentChat.GetChat(ctx, "chat1")
	require.Error(t, err)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)

	// chat2, bound to the untouched w2, survives with its PTY left alone.
	chat2, err := c.AgentChat.GetChat(ctx, "chat2")
	require.NoError(t, err)
	assert.Equal(t, "w2", chat2.WorkspaceID)
	assert.NotContains(t, term.terminated(), "term-2")
}

// TestContainer_WireCallbacks_DeleteCascade_ForgetsAgentChats_NilTerminateSession
// pins that a nil terminateSession (the zero value most tests in this file use)
// degrades to "forget with no PTY teardown" rather than panicking — production
// always injects a real seam (app.terminateAgentSession), but a nil must stay
// safe for any caller that does not.
func TestContainer_WireCallbacks_DeleteCascade_ForgetsAgentChats_NilTerminateSession(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	c, err := repositories.New(ctx, ad, hub.NewHub(), ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, nil)
	require.NoError(t, err)

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	createAgentChat(t, ctx, c.AgentChat, "chat1", "w1", "term-1")
	c.WaitQuiescent()

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	_, err = c.AgentChat.GetChat(ctx, "chat1")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestContainer_WireCallbacks_DeleteCascade_TerminateFailure_IsBestEffort pins
// Task 12 review Important 1 at the cascade layer: a terminate failure for one
// chat's PTY must NOT abort the cascade — it is logged and the chat is Forgotten
// anyway, AND the remaining chats in the workspace are still processed. Wedging
// the whole workspace delete (worktree never reaped) on a terminate error the
// cascade can't re-drive is a far worse outcome than an orphaned PTY.
func TestContainer_WireCallbacks_DeleteCascade_TerminateFailure_IsBestEffort(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	term := &fakeTerminateSession{failFor: "term-1"} // chat1's PTY terminate fails
	c, err := repositories.New(ctx, ad, hub.NewHub(), ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, term.terminate)
	require.NoError(t, err)

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	createAgentChat(t, ctx, c.AgentChat, "chat1", "w1", "term-1")
	createAgentChat(t, ctx, c.AgentChat, "chat2", "w1", "term-2")
	c.WaitQuiescent()

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	// Both PTYs were attempted (chat1's failed) ...
	assert.ElementsMatch(t, []string{"term-1", "term-2"}, term.terminated())
	// ... and BOTH chats are Forgotten despite chat1's terminate failure — the
	// failure neither aborted chat1's own Forget nor stopped chat2 from being
	// processed.
	_, err = c.AgentChat.GetChat(ctx, "chat1")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
	_, err = c.AgentChat.GetChat(ctx, "chat2")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestContainer_WireCallbacks_DeleteCascade_ForgetsSoftDeletedChats pins Task 12
// review Important 2: a chat already SOFT-deleted (Status==deleted, e.g. via the
// usecase's DeleteChat) is invisible to the live-filtered ListByWorkspace, so
// the cascade must enumerate via ListByWorkspaceIncludingDeleted — otherwise the
// tombstoned chat's event log + read row would survive the deleted workspace
// forever. Here chat1 is soft-deleted before the workspace delete; the cascade
// must still hard-Forget it.
func TestContainer_WireCallbacks_DeleteCascade_ForgetsSoftDeletedChats(t *testing.T) {
	ctx := context.Background()
	ad := newAdapter(t)
	c, err := repositories.New(ctx, ad, hub.NewHub(), ax[domain.ReviewThread](t), wsAx(t, ad), agentChatAx(t, ad), nil, nil)
	require.NoError(t, err)

	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	createAgentChat(t, ctx, c.AgentChat, "chat1", "w1", "term-1")
	c.WaitQuiescent()

	// Soft-delete chat1: it is now a tombstone hidden from ListByWorkspace but
	// still anchored to w1 and still holding an event log + read row.
	require.NoError(t, c.AgentChat.Delete(ctx, "chat1"))
	c.WaitQuiescent()
	live, err := c.AgentChat.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	require.Empty(t, live, "the soft-deleted chat must be hidden from the live-filtered list")

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))
	c.WaitQuiescent()
	c.Drain().WG.Wait()
	c.WaitQuiescent()

	// The soft-deleted chat is genuinely gone: the cascade hard-Forgot it even
	// though the live filter hid it.
	_, err = c.AgentChat.GetChat(ctx, "chat1")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}
