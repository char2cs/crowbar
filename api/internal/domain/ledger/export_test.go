package ledger

import (
	"maps"
	"slices"
)

// RowKinds is every row kind Check accepts, sorted.
func RowKinds() []Kind { return slices.Sorted(maps.Keys(checks)) }

// MarkerKinds is every marker kind Check accepts, sorted.
func MarkerKinds() []string { return slices.Sorted(slices.Values(markerKinds)) }
