package app

import (
	"context"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

// newAsynx builds the shared asynx wiring every aggregate type gets. opts run
// last, right before Build, so a caller can layer aggregate-specific config
// (WithSchemaVersion/WithUpcaster — see workspace.go's retired-field upcaster
// for why Workspace is the one that currently needs this) onto the same
// defaults every other aggregate gets.
func newAsynx[T any](
	es asynxModels.Store,
	ss asynxModels.SnapshotStore,
	opts ...func(*asynx.Builder[T]),
) (asynx.Asynx[T], error) {
	b := asynx.New[T]().
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
		})
	for _, opt := range opts {
		opt(b)
	}
	return b.Build()
}
