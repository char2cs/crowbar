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

// newProjected registers BOTH the save-only store projection and the hub broadcast
// projection on a real asynx over throwaway DBs — the production shape (one of each
// per singleton).
func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.ReviewThread], storage, *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ReviewThread]().
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
	require.NoError(t, registerStoreProjection(st, ax))
	require.NoError(t, registerHubProjection(ax, h.push))
	return context.Background(), ax, st, h
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

func TestStoreProjection_ForgetDeletesRow(t *testing.T) {
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
	subscribeCnt int
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.ReviewThread],
	_ ...asynxModels.SubscriptionOpt[domain.ReviewThread],
) (string, error) {
	f.subscribeCnt++
	if f.subscribeCnt == 1 {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.ReviewThread],
) (string, error) {
	return "", f.forgetErr
}

func TestRegisterStoreProjection_SubscribeError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	err = registerStoreProjection(st, &fakeAx{subscribeErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store projection: subscribe")
}

func TestRegisterStoreProjection_OnForgetError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	err = registerStoreProjection(st, &fakeAx{forgetErr: errors.New("bus down")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store projection: on forget")
}

func TestRegisterHubProjection_SubscribeError(t *testing.T) {
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread hub projection: subscribe")
}

func TestStoreProjector_OnEvent_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &storeProjector{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	p.onEvent(ctx, asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: "t1"}})
}

func TestStoreProjector_OnForget_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &storeProjector{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	p.onForget(ctx, asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: "t1"}})
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	_, err = New(db, nil, ax, func(domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store: projections")
}
