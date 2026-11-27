package ledger

import (
	"maps"
	"slices"
)

func RowKinds() []Kind { return slices.Sorted(maps.Keys(checks)) }

func MarkerKinds() []string { return slices.Sorted(slices.Values(markerKinds)) }
