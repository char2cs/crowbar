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
	require.NoError(t, registerProjections(st, ax, h.push))
	return context.Background(), ax, st, h
}

func TestProjection_CreateUpsertsRowAndBroadcasts(t *testing.T) {
	ctx, ax, st, h := newProjected(t)
	_, err := ax.SendWait(ctx, rtcmds.OpenReviewThread{
		ID: "t1", WsID: "w1", MessageID: "m1", FilePath: "a.go", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "a.go", row.FilePath)
	assert.GreaterOrEqual(t, h.count(), 1)
}

func TestProjection_ForgetDeletesRow(t *testing.T) {
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

// fakeAx is a minimal asynx stub that fails Subscribe on demand.
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

func TestRegisterProjections_SubscribeError(t *testing.T) {
	ctx := context.Background()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	err = registerProjections(st, ax, func(_ domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread projection: subscribe")
	_ = ctx
}

func TestRegisterProjections_OnForgetError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: nil, forgetErr: errors.New("bus down")}
	err = registerProjections(st, ax, func(_ domain.ReviewThread) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread projection: on forget")
}

func TestProjection_OnEvent_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.ReviewThread) {},
	}
	p.onEvent(ctx, asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: "t1"}})
}

func TestProjection_OnForget_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.ReviewThread) {},
	}
	p.onForget(ctx, asynxModels.Event[domain.ReviewThread]{Aggregate: domain.ReviewThread{ID: "t1"}})
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: errors.New("bus down")}
	_, err = New(context.Background(), db, ax, func(domain.ReviewThread) {}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reviewthread store: projections")
}
