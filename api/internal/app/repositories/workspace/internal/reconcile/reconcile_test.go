package reconcile

import (
	"context"
	"errors"
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
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	<-entered
	close(release)
	gate.WaitIdle(context.Background())

	assert.Equal(t, int32(1), atomic.LoadInt32(&deriveCalls), "repeat opens must not stack tasks")
	assert.Equal(t, int32(1), atomic.LoadInt32(&sendCalls))
	assert.Equal(t, "workspace.working_tree_synced.w1", gotName)
	require.Len(t, spy.all(), 1, "exactly one corrected frame broadcast")
	assert.Equal(t, "w1", spy.all()[0].ID)
}

func TestReconciler_OnOpen_SkipsOnNotFound(t *testing.T) {
	derive := func(
		_ context.Context,
		_ string,
	) (asynxModels.Command[domain.Workspace], error) {
		return nil, fmt.Errorf("gone: %w", problem.ErrNotFound)
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

func TestWithTimeout_OnlyAcceptsAPositiveDuration(t *testing.T) {
	gate := drain.New()

	r := New(nil, nil, nil, gate, WithTimeout(5*time.Second))
	assert.Equal(t, 5*time.Second, r.timeout)

	ignored := New(nil, nil, nil, gate, WithTimeout(0), WithTimeout(-time.Second))
	assert.Equal(t, defaultReconcileTimeout, ignored.timeout, "a non-positive duration must be ignored, not adopted")
}

func TestReconciler_OnOpen_RefusedOnceDraining(t *testing.T) {
	gate := drain.New()
	gate.Wait(context.Background())

	var deriveCalls int32
	derive := func(context.Context, string) (asynxModels.Command[domain.Workspace], error) {
		atomic.AddInt32(&deriveCalls, 1)
		return nil, nil
	}
	r := New(derive, nil, nil, gate)

	r.OnOpen(context.Background(), "w1")

	assert.Zero(t, atomic.LoadInt32(&deriveCalls), "a refused claim must not spawn the reconcile task")
	r.mu.Lock()
	_, stillClaimed := r.inflight["w1"]
	r.mu.Unlock()
	assert.False(t, stillClaimed, "a refused claim must be released, or this id can never reconcile again")
}

func TestReconciler_Reconcile_DeriveErrorAbortsBeforeSend(t *testing.T) {
	derive := func(context.Context, string) (asynxModels.Command[domain.Workspace], error) {
		return nil, errors.New("git worktree unreadable")
	}
	var sendCalls int32
	sendWait := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
		atomic.AddInt32(&sendCalls, 1)
		return asynxModels.Event[domain.Workspace]{}, nil
	}
	spy := &broadcastSpy{}
	r := New(derive, sendWait, spy.record, drain.New())

	r.reconcile(context.Background(), "w1")

	assert.Zero(t, atomic.LoadInt32(&sendCalls), "a real derive error must never reach SendWait")
	assert.Empty(t, spy.all())
}

func TestReconciler_Reconcile_SendWaitErrorAbortsBroadcast(t *testing.T) {
	derive := func(_ context.Context, wsID string) (asynxModels.Command[domain.Workspace], error) {
		return wscmds.SyncWorkingTreeState{ID: wsID, Now: time.Unix(2, 0).UTC()}, nil
	}
	sendWait := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
		return asynxModels.Event[domain.Workspace]{}, errors.New("event store unavailable")
	}
	spy := &broadcastSpy{}
	r := New(derive, sendWait, spy.record, drain.New())

	r.reconcile(context.Background(), "w1")

	assert.Empty(t, spy.all(), "a real SendWait error must never reach broadcast")
}

func newAx(
	t *testing.T,
	st *projections.Store,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	if st != nil {
		projections.Subscriber(st)(b)
	}
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), ax
}

func TestReconciler_OnOpen_UpdatesReadModelAndBroadcasts(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := projections.NewStore(db)
	require.NoError(t, err)
	ctx, ax := newAx(t, st)

	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID:           "w1",
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "main",
		Now:          time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
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
