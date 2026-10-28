package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

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

// healable is a node store over one log, with the read-model DB to lose rows from.
func healable(t *testing.T) (context.Context, Store, asynxModels.Store, *gormdb.DB) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	p, err := Open(db)
	require.NoError(t, err)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	b := asynx.New[domain.Node]().WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	Subscriber(p)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	st, err := New(p, es, ax, func(NodeEvent) {})
	require.NoError(t, err)
	ctx := context.Background()
	_, err = ax.SendWait(ctx, nodecmds.Create{ID: "n1", Kind: domain.NodeKindChat, ParentID: "p1"})
	require.NoError(t, err)
	require.NoError(t, db.WithContext(ctx).Exec("DELETE FROM nodes_read").Error)
	return ctx, st, es, db
}

func TestStore_AHealThatCannotSaveIsAnError(t *testing.T) {
	ctx, st, _, db := healable(t)
	require.NoError(t, db.WithContext(ctx).Exec("PRAGMA query_only = ON").Error)

	_, err := st.GetNode(ctx, "n1")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrNotFound, "a row that could not be saved is not a missing node")
	_, err = st.ListAll(ctx)
	require.Error(t, err)
}

func TestStore_AHealNeverResurrectsATombstonedNode(t *testing.T) {
	ctx, st, es, _ := healable(t)
	appendTombstone(t, es, "n1")

	_, err := st.GetNode(ctx, "n1")
	require.ErrorIs(t, err, ErrNotFound)
	all, err := st.ListAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

// appendTombstone writes the marker a Forget leaves when it fails before the erase.
func appendTombstone(t *testing.T, es asynxModels.Store, id string) {
	t.Helper()
	ctx := context.Background()
	prior, err := es.ReadFrom(ctx, "events:"+id, 1)
	require.NoError(t, err)
	var last struct {
		SchemaVersion int `json:"schema_version"`
	}
	require.NoError(t, json.Unmarshal(prior[len(prior)-1], &last))
	v := int64(len(prior) + 1)
	evt, err := json.Marshal(map[string]any{
		"id": "tomb-" + id, "event_name": asynxModels.ForgetEventName, "version": v,
		"schema_version": last.SchemaVersion, "occurred_at": time.Unix(2, 0).UTC(), "patches": json.RawMessage("[]"),
	})
	require.NoError(t, err)
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error { return tx.Append(ctx, "events:"+id, v, evt) }))
}
