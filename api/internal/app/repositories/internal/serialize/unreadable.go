package serialize

import (
	"context"
	"log/slog"
	"sync"
)

// Unreadable is the set of read-model rows a list could not decode. One bad row
// does not fail its list, and is logged once per process, not once per read.
// The zero value is ready to use.
type Unreadable struct {
	warned sync.Map
}

// Skip records that kind row id would not decode, logging it the first time.
func (u *Unreadable) Skip(ctx context.Context, kind, id string, err error) {
	if _, seen := u.warned.LoadOrStore(id, struct{}{}); seen {
		return
	}
	slog.ErrorContext(ctx, "read model: a row will not decode", "kind", kind, "id", id, "err", err)
}
