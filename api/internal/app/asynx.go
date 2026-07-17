package app

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

func newAsynx[T any](
	es asynxModels.Store,
	ss asynxModels.SnapshotStore,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		// Surface dropped projections instead of letting the read model rot
		// silently: a projection panic or a publish error means the event is
		// durable but the read-model row may be stale until the next
		// reconcile-on-open (see store.New). Without these handlers asynx swallows
		// both, so the divergence is invisible.
		WithPanicHandler(func(ctx context.Context, evt asynxModels.Event[T], p any) {
			slog.ErrorContext(ctx, "asynx projection panic; read model may be stale until reconcile",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "panic", p)
		}).
		WithPublishErrorHandler(func(ctx context.Context, evt asynxModels.Event[T], err error) {
			slog.ErrorContext(ctx, "asynx publish error; read model may be stale until reconcile",
				"aggregate", evt.AggregateID, "event", evt.EventName, "version", evt.Version, "err", err)
		}).
		Build()
}
