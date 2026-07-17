package reconcile

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
)

type broadcastSpy struct {
	mu   sync.Mutex
	rows []domain.Workspace
}

func (b *broadcastSpy) record(
	ws domain.Workspace,
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rows = append(b.rows, ws)
}

func (b *broadcastSpy) all() []domain.Workspace {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]domain.Workspace(nil), b.rows...)
}

// TestReconciler_OnOpen_DedupsRepeatOpens proves the §3.8 dedup contract: while a
// reconcile task for one id is in flight, repeat opens do NOT stack additional
// tasks. The derive func blocks until the test releases it, so all five opens
// race against a single in-flight task; only one derivation + one SendWait + one
// broadcast may run.
func TestReconciler_OnOpen_DedupsRepeatOpens(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	var deriveCalls int32
	derive := func(
		_ context.Context,
		wsID string,
	) (asynxModels.Command[domain.Workspace], error) {
		atomic.AddInt32(&deriveCalls, 1)
		entered <- struct{}{}
		<-release
		return wscmds.SyncWorkingTreeState{ID: wsID, Added: 3, Now: time.Unix(2, 0).UTC()}, nil
	}
	var sendCalls int32
	var gotName string
	sendWait := func(
		_ context.Context,
		cmd asynxModels.Command[domain.Workspace],
	) (asynxModels.Event[domain.Workspace], error) {
		atomic.AddInt32(&sendCalls, 1)
		gotName = cmd.EventName()
		return asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: cmd.AggregateID(), Added: 3}}, nil
	}
	spy := &broadcastSpy{}
	gate := drain.New()
	r := New(derive, sendWait, spy.record, gate)

	ctx := context.Background()
	for range 5 {
		r.OnOpen(ctx, "w1")
	}
	// Block until the single in-flight task has actually entered derive — dedup
	// guarantees exactly one does — then release it. No polling, no timeout.
	<-entered
	close(release)
	gate.WaitIdle(context.Background())

	assert.Equal(t, int32(1), atomic.LoadInt32(&deriveCalls), "repeat opens must not stack tasks")
	assert.Equal(t, int32(1), atomic.LoadInt32(&sendCalls))
	assert.Equal(t, "workspace.working_tree_synced.w1", gotName)
	require.Len(t, spy.all(), 1, "exactly one corrected frame broadcast")
	assert.Equal(t, "w1", spy.all()[0].ID)
}

// TestReconciler_OnOpen_SkipsOnNotFound proves idempotency: a vanished workspace
// (derive returns apperr.ErrNotFound) is a no-op — no command is sent and no
// frame is broadcast (spec §3.8 "skip on ErrNotFound").
func TestReconciler_OnOpen_SkipsOnNotFound(t *testing.T) {
	derive := func(
		_ context.Context,
		_ string,
	) (asynxModels.Command[domain.Workspace], error) {
		return nil, fmt.Errorf("gone: %w", apperr.ErrNotFound)
	}
	var sendCalls int32
	sendWait := func(
		_ context.Context,
		_ asynxModels.Command[domain.Workspace],
	) (asynxModels.Event[domain.Workspace], error) {
		atomic.AddInt32(&sendCalls, 1)
		return asynxModels.Event[domain.Workspace]{}, nil
	}
	spy := &broadcastSpy{}
	gate := drain.New()
	r := New(derive, sendWait, spy.record, gate)

	r.OnOpen(context.Background(), "w1")
	gate.WaitIdle(context.Background())

	assert.Zero(t, atomic.LoadInt32(&sendCalls), "a vanished workspace must not be re-sent")
	assert.Empty(t, spy.all(), "a vanished workspace must not broadcast")
}

// TestReconciler_OnOpen_SkipsBroadcastOnValidation proves that a non-applicable
// transition (SendWait rejects the pure command with ErrValidation) is an
// idempotent no-op: nothing is broadcast (spec §3.8 "Validate guards reject
// non-applicable transitions").
func TestReconciler_OnOpen_SkipsBroadcastOnValidation(t *testing.T) {
	derive := func(
		_ context.Context,
		wsID string,
	) (asynxModels.Command[domain.Workspace], error) {
		return wscmds.SyncWorkingTreeState{ID: wsID, Now: time.Unix(2, 0).UTC()}, nil
	}
	sendWait := func(
		_ context.Context,
		_ asynxModels.Command[domain.Workspace],
	) (asynxModels.Event[domain.Workspace], error) {
		return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("rejected: %w", asynxModels.ErrValidation)
	}
	spy := &broadcastSpy{}
	gate := drain.New()
	r := New(derive, sendWait, spy.record, gate)

	r.OnOpen(context.Background(), "w1")
	gate.WaitIdle(context.Background())

	assert.Empty(t, spy.all(), "a rejected transition must not broadcast")
}

// TestReconciler_OnOpen_IgnoresEmptyID guards against a blank id dispatching a
// task (a create that has not produced an aggregate yet).
func TestReconciler_OnOpen_IgnoresEmptyID(t *testing.T) {
	var deriveCalls int32
	derive := func(
		_ context.Context,
		_ string,
	) (asynxModels.Command[domain.Workspace], error) {
		atomic.AddInt32(&deriveCalls, 1)
		return nil, nil
	}
	gate := drain.New()
	r := New(derive, nil, nil, gate)

	r.OnOpen(context.Background(), "")
	gate.WaitIdle(context.Background())

	assert.Zero(t, atomic.LoadInt32(&deriveCalls))
}

func newAx(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), ax
}

// TestReconciler_OnOpen_UpdatesReadModelAndBroadcasts drives a real axWorkspace
// with the real save-only store projection: the reconcile task re-derives reality
// (here Added=7), SendWaits the pure sync command (which the store projection
// folds into state/store/workspace.db), then broadcasts the corrected frame. This
// is the end-to-end "reconcile updates the read model + broadcasts" of §3.8.
func TestReconciler_OnOpen_UpdatesReadModelAndBroadcasts(t *testing.T) {
	ctx, ax := newAx(t)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := projections.NewStore(db)
	require.NoError(t, err)
	require.NoError(t, projections.RegisterStore(st, ax))

	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "main",
		Now:       time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	derive := func(
		_ context.Context,
		wsID string,
	) (asynxModels.Command[domain.Workspace], error) {
		return wscmds.SyncWorkingTreeState{ID: wsID, Added: 7, Now: time.Unix(2, 0).UTC()}, nil
	}
	spy := &broadcastSpy{}
	gate := drain.New()
	r := New(derive, ax.SendWait, spy.record, gate)

	r.OnOpen(ctx, "w1")
	gate.WaitIdle(context.Background())

	got, err := st.Get(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 7, got.Added, "the read model must reflect the re-derived reality")
	require.Len(t, spy.all(), 1)
	assert.Equal(t, 7, spy.all()[0].Added, "the broadcast frame must carry the corrected state")
}
