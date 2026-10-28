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
	nodecmds "github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func openProjection(t *testing.T) *Projection {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	return p
}

// buildOver builds axNode with p as its durable subscriber, the production shape.
func buildOver(t *testing.T, p *Projection) (context.Context, asynx.Asynx[domain.Node]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		WithDeliveryRetry(time.Millisecond, time.Millisecond)
	Subscriber(p)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	return context.Background(), ax
}

// failFirstSave fails the first Save, then signals every Save that succeeds.
type failFirstSave struct {
	storage
	failed atomic.Bool
	saved  chan struct{}
}

func (f *failFirstSave) Save(ctx context.Context, n domain.Node) error {
	if f.failed.CompareAndSwap(false, true) {
		return errors.New("injected save failure")
	}
	if err := f.storage.Save(ctx, n); err != nil {
		return err
	}
	f.saved <- struct{}{}
	return nil
}

func TestNodeProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) {
	p := openProjection(t)
	flaky := &failFirstSave{storage: p.storage, saved: make(chan struct{}, 16)}
	p.storage = flaky
	ctx, ax := buildOver(t, p)

	_, err := ax.SendWait(ctx, nodecmds.Create{ID: "n1", Kind: domain.NodeKindChat, ParentID: "p1"})
	require.NoError(t, err)

	select {
	case <-flaky.saved:
	case <-time.After(10 * time.Second):
		t.Fatal("the failed save was never retried")
	}
	got, err := flaky.FindByKey(ctx, "n1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "p1", got.ParentID)
}

func TestNodeProjection_ForgetDeletesTheRow(t *testing.T) {
	p := openProjection(t)
	ctx, ax := buildOver(t, p)
	_, err := ax.SendWait(ctx, nodecmds.Create{ID: "n1", Kind: domain.NodeKindChat, ParentID: "p1"})
	require.NoError(t, err)

	require.NoError(t, ax.Forget(ctx, "n1"))

	got, err := p.storage.FindByKey(ctx, "n1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// Upstream can redeliver a Forget marker at startup; the second one finds no row.
func TestNodeProjection_AForgetDeliveredTwiceIsANoOp(t *testing.T) {
	p := openProjection(t)
	ctx := context.Background()
	require.NoError(t, p.storage.Save(ctx, domain.Node{ID: "n1", ParentID: "p1"}))
	marker := asynxModels.Event[domain.Node]{AggregateID: "n1", EventName: asynxModels.ForgetEventName}

	require.NoError(t, p.handle(ctx, marker))
	require.NoError(t, p.handle(ctx, marker))

	got, err := p.storage.FindByKey(ctx, "n1")
	require.NoError(t, err)
	assert.Nil(t, got)
}
