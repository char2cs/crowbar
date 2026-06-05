package app

import (
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

func newAsynx[T any](
	es asynxModels.Store,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}
