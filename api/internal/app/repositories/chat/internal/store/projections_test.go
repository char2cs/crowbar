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
	chatcmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type captureHub struct {
	mu   sync.Mutex
	rows []domain.Chat
}

func (h *captureHub) push(c domain.Chat) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rows = append(h.rows, c)
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rows)
}

func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Chat], storage, *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
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
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{
		ID: "c1", WsID: "w1", Title: "hello", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "hello", row.Title)
	assert.GreaterOrEqual(t, h.count(), 1)
}

func TestProjection_ForgetDeletesRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, chatcmds.CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "c1"))

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

// fakeAx is a minimal asynx stub that fails Subscribe on demand.
type fakeAx struct {
	asynx.Asynx[domain.Chat]
	subscribeErr error
	forgetErr    error
	subscribeCnt int
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Chat],
	_ ...asynxModels.SubscriptionOpt[domain.Chat],
) (string, error) {
	f.subscribeCnt++
	if f.subscribeCnt == 1 {
		return "", f.subscribeErr
	}
	return "sub-id", nil
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.Chat],
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
	err = registerProjections(st, ax, func(_ domain.Chat) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat projection: subscribe")
	_ = ctx
}

func TestRegisterProjections_OnForgetError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	ax := &fakeAx{subscribeErr: nil, forgetErr: errors.New("bus down")}
	err = registerProjections(st, ax, func(_ domain.Chat) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat projection: on forget")
}

func TestProjection_OnEvent_SaveError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.Chat) {},
	}
	p.onEvent(ctx, asynxModels.Event[domain.Chat]{Aggregate: domain.Chat{ID: "c1"}})
}

func TestProjection_OnForget_DeleteError_DoesNotPanic(t *testing.T) {
	ctx := context.Background()
	p := &projector{
		store:     &storageStore{inner: &errInner{err: errors.New("db down")}},
		broadcast: func(_ domain.Chat) {},
	}
	p.onForget(ctx, asynxModels.Event[domain.Chat]{Aggregate: domain.Chat{ID: "c1"}})
}
