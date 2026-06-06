package store

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

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	arcmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type captureHub struct {
	mu   sync.Mutex
	rows []domain.AgentRun
}

func (h *captureHub) push(run domain.AgentRun) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rows = append(h.rows, run)
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rows)
}

func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.AgentRun], storage, *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRun]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)

	h := &captureHub{}
	require.NoError(t, registerProjections(st, ax, h.push))
	return context.Background(), ax, st, h
}

func TestProjection_CreateUpsertsRowAndBroadcasts(t *testing.T) {
	ctx, ax, st, h := newProjected(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{
		ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, domain.AgentRunStatusPending, row.Status)
	assert.GreaterOrEqual(t, h.count(), 1)
}

func TestProjection_ForgetDeletesRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, arcmds.CreateAgentRun{
		ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "a1"))

	row, err := st.FindByKey(ctx, "a1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

// fakeAx is a minimal asynx stub that fails Subscribe on demand.
type fakeAx struct {
	asynx.Asynx[domain.AgentRun]
	subscribeErr error
	forgetErr    error
	subscribeCnt int
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.AgentRun],
	_ ...asynxModels.SubscriptionOpt[domain.AgentRun],
) (string, error) {
	f.subscribeCnt++
	if f.subscribeCnt == 1 {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.AgentRun],
) (string, error) {
	return "", f.forgetErr
}

func TestRegisterProjections_SubscribeError(t *testing.T) {
	ctx := context.Background()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	err = registerProjections(st, ax, func(_ domain.AgentRun) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrun projection: subscribe")
	_ = ctx
}

func TestRegisterProjections_OnForgetError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: nil, forgetErr: errors.New("bus down")}
	err = registerProjections(st, ax, func(_ domain.AgentRun) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrun projection: on forget")
}

func TestProjection_OnEvent_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.AgentRun) {},
	}
	p.onEvent(ctx, asynxModels.Event[domain.AgentRun]{Aggregate: domain.AgentRun{ID: "a1"}})
}

func TestProjection_OnForget_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.AgentRun) {},
	}
	p.onForget(ctx, asynxModels.Event[domain.AgentRun]{Aggregate: domain.AgentRun{ID: "a1"}})
}

type flakyStorage struct {
	storage
	failCount int
	calls     int
	err       error
}

func (f *flakyStorage) Save(
	_ context.Context,
	_ domain.AgentRun,
) error {
	f.calls++
	if f.calls <= f.failCount {
		return f.err
	}
	return nil
}

func TestSaveWithRetry_RetriesTransientThenSucceeds(t *testing.T) {
	st := &flakyStorage{failCount: 2, err: errors.New("disk I/O error")}
	p := &projector{store: st, broadcast: func(_ domain.AgentRun) {}}
	err := p.saveWithRetry(context.Background(), domain.AgentRun{ID: "a1"})
	require.NoError(t, err)
	assert.Equal(t, 3, st.calls)
}

func TestSaveWithRetry_PersistentTransientGivesUp(t *testing.T) {
	st := &flakyStorage{failCount: 5, err: errors.New("disk I/O error")}
	p := &projector{store: st, broadcast: func(_ domain.AgentRun) {}}
	err := p.saveWithRetry(context.Background(), domain.AgentRun{ID: "a1"})
	require.Error(t, err)
	assert.Equal(t, 3, st.calls)
}

func TestSaveWithRetry_NonTransientNotRetried(t *testing.T) {
	st := &flakyStorage{failCount: 5, err: errors.New("constraint violation")}
	p := &projector{store: st, broadcast: func(_ domain.AgentRun) {}}
	err := p.saveWithRetry(context.Background(), domain.AgentRun{ID: "a1"})
	require.Error(t, err)
	assert.Equal(t, 1, st.calls)
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	_, err = New(db, ax, func(domain.AgentRun) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentrun store: projections")
}
