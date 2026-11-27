package serialize

import "context"

type AggregateLister interface {
	AggregateIDs(
		ctx context.Context,
	) ([]string, error)
}
