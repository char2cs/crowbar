package store

import (
	"context"
	"errors"
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
	rtcmds "github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type captureHub struct {
	mu   sync.Mutex
	rows []domain.ReviewThread
}

func (h *captureHub) push(th domain.ReviewThread) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rows = append(h.rows, th)
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rows)
}

// newProjected builds a real asynx over throwaway DBs with the store projection
// as its durable subscriber and the hub projection live, the production shape.
func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.ReviewThread], storage, *captureHub) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	ctx, ax, h := buildOver(t, p)
	return ctx, ax, p.storage, h
}

func buildOver(
	t *testing.T,
	p *Projection,
) (context.Context, asynx.Asynx[domain.ReviewThread], *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.ReviewThread]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithDeliveryRetry(time.Millisecond, time.Millisecond)
	Subscriber(p)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	h := &captureHub{}
	require.NoError(t, registerHubProjection(ax, h.push))
	return context.Background(), ax, h
}

// failFirstSave fails the first Save, then signals every Save that succeeds.
type failFirstSave struct {
	storage
	failed atomic.Bool
	saved  chan struct{}
}

func (f *failFirstSave) Save(ctx context.Context, th domain.ReviewThread) error {
	if f.failed.CompareAndSwap(false, true) {
		return errors.New("injected save failure")
	}
	if err := f.storage.Save(ctx, th); err != nil {
		return err
	}
	f.saved <- struct{}{}
	return nil
}

func TestStoreProjection_CreateUpsertsRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "a.go", row.FilePath)
}

func TestHubProjection_CreateBroadcastsBareAggregate(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, h.count(), 1, "hub projection must broadcast the frame")
}

func TestReviewThreadProjection_ForgetDeletesTheRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "t1"))

	row, err := st.FindByKey(ctx, "t1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

// fakeAx is a minimal asynx stub that fails Subscribe / OnForget on demand.
type fakeAx struct {
	asynx.Asynx[domain.ReviewThread]
	subscribeErr error
	forgetErr    error
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.ReviewThread],
	_ ...asynxModels.SubscriptionOpt[domain.ReviewThread],
) (string, error) {
	return "", f.subscribeErr
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.ReviewThread],
) (string, error) {
	return "", f.forgetErr
}

func TestRegisterHubProjection_SubscribeError(t *testing.T) {
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread hub projection: subscribe")
}

func TestReviewThreadProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	flaky := &failFirstSave{storage: p.storage, saved: make(chan struct{}, 16)}
	p.storage = flaky
	ctx, ax, _ := buildOver(t, p)

	_, err = ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	select {
	case <-flaky.saved:
	case <-time.After(10 * time.Second):
		t.Fatal("the failed save was never retried")
	}
	row, err := flaky.FindByKey(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "a.go", row.FilePath)
}

// Upstream can redeliver a Forget marker at startup; the second one finds no row.
func TestReviewThreadProjection_AForgetDeliveredTwiceIsANoOp(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, p.storage.Save(ctx, domain.ReviewThread{ID: "t1", WsID: "w1"}))
	marker := asynxModels.Event[domain.ReviewThread]{AggregateID: "t1", EventName: asynxModels.ForgetEventName}

	require.NoError(t, p.handle(ctx, marker))
	require.NoError(t, p.handle(ctx, marker))

	row, err := p.storage.FindByKey(ctx, "t1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestReviewThreadProjection_AStorageFailureIsReturned(t *testing.T) {
	ctx := context.Background()
	p := &Projection{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	require.Error(t, p.handle(ctx, asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: "t1"}}))
	assert.Error(t, p.handle(ctx, asynxModels.Event[domain.ReviewThread]{AggregateID: "t1", EventName: asynxModels.ForgetEventName}))
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	_, err = New(p, nil, &fakeAx{subscribeErr: errors.New("bus down")}, func(domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store: projections")
}
