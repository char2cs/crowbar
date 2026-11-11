package serialize

import "context"

// AggregateLister is the optional event-store capability that drives
// reconcile-on-open: each listed id is replayed to heal rows a dropped projection
// missed. A store that does not implement it skips reconcile.
type AggregateLister interface {
	AggregateIDs(
		ctx context.Context,
	) ([]string, error)
}
