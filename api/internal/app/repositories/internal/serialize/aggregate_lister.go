package serialize

import "context"

// AggregateLister is the optional capability a global event store exposes so a
// read model can enumerate every aggregate it holds. The chat and reviewthread
// repositories type-assert their injected event store to this interface to drive
// reconcile-on-open: the read model replays each id from the event log, healing
// rows a dropped projection left missing. An event store that does not implement
// it simply skips reconcile (best-effort).
type AggregateLister interface {
	AggregateIDs(
		ctx context.Context,
	) ([]string, error)
}
