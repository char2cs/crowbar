package store

import (
	"context"
	"errors"
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
	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newProjected builds a real asynx over throwaway DBs with the store projection
// as its durable subscriber and the hub projection live, the production shape.
func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Chat], storage, *captureHub) {
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
) (context.Context, asynx.Asynx[domain.Chat], *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithDeliveryRetry(time.Millisecond, time.Millisecond)
	Subscriber(p)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	h := &captureHub{}
	require.NoError(t, registerHubProjection(ax, h.watch))
	return context.Background(), ax, h
}

// failFirstSave fails the first Save, then signals every Save that succeeds.
type failFirstSave struct {
	storage
	failed atomic.Bool
	saved  chan struct{}
}

func (f *failFirstSave) Save(ctx context.Context, chat domain.Chat) error {
	if f.failed.CompareAndSwap(false, true) {
		return errors.New("injected save failure")
	}
	if err := f.storage.Save(ctx, chat); err != nil {
		return err
	}
	f.saved <- struct{}{}
	return nil
}

func TestStoreProjection_CreateUpsertsRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, accmds.Create{
		ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "w1", row.WorkspaceID)
}

func TestChatProjection_ForgetDeletesTheRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, accmds.Create{
		ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "c1"))

	row, err := st.FindByKey(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestChatProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	flaky := &failFirstSave{storage: p.storage, saved: make(chan struct{}, 16)}
	p.storage = flaky
	ctx, ax, _ := buildOver(t, p)

	_, err = ax.SendWait(ctx, accmds.Create{
		ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	select {
	case <-flaky.saved:
	case <-time.After(10 * time.Second):
		t.Fatal("the failed save was never retried")
	}
	row, err := flaky.FindByKey(ctx, "c1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "w1", row.WorkspaceID)
}

// Upstream can redeliver a Forget marker at startup; the second one finds no row.
func TestChatProjection_AForgetDeliveredTwiceIsANoOp(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, p.storage.Save(ctx, domain.Chat{ID: "c1", WorkspaceID: "w1"}))
	marker := asynxModels.Event[domain.Chat]{AggregateID: "c1", EventName: asynxModels.ForgetEventName}

	require.NoError(t, p.handle(ctx, marker))
	require.NoError(t, p.handle(ctx, marker))

	row, err := p.storage.FindByKey(ctx, "c1")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestChatProjection_AStorageFailureIsReturned(t *testing.T) {
	ctx := context.Background()
	p := &Projection{storage: &storageStore{inner: &errInner{err: errors.New("db down")}}}
	require.Error(t, p.handle(ctx, asynxModels.Event[domain.Chat]{Aggregate: domain.Chat{ID: "c1"}}))
	assert.Error(t, p.handle(ctx, asynxModels.Event[domain.Chat]{AggregateID: "c1", EventName: asynxModels.ForgetEventName}))
}

// fakeAxFailsHubSubscribe fails the hub projection's Subscribe.
type fakeAxFailsHubSubscribe struct {
	asynx.Asynx[domain.Chat]
}

func (f *fakeAxFailsHubSubscribe) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Chat],
	_ ...asynxModels.SubscriptionOpt[domain.Chat],
) (string, error) {
	return "", errors.New("bus down for hub")
}

func TestNew_HubProjectionError(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	_, err = New(p, nil, &fakeAxFailsHubSubscribe{}, func(ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat store: projections")
}

// fakeAx is a minimal asynx stub that fails Subscribe / OnForget on demand.
type fakeAx struct {
	asynx.Asynx[domain.Chat]
	subscribeErr error
	forgetErr    error
}

func (f *fakeAx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Chat],
	_ ...asynxModels.SubscriptionOpt[domain.Chat],
) (string, error) {
	return "", f.subscribeErr
}

func (f *fakeAx) OnForget(
	_ asynxModels.ForgetHandler[domain.Chat],
) (string, error) {
	return "", f.forgetErr
}
