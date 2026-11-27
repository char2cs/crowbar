package serialize

import (
	"context"
	"log/slog"
	"sync"
)

type Unreadable struct {
	warned sync.Map
}

func (u *Unreadable) Skip(ctx context.Context, kind, id string, err error) {
	if _, seen := u.warned.LoadOrStore(id, struct{}{}); seen {
		return
	}
	slog.ErrorContext(ctx, "read model: a row will not decode", "kind", kind, "id", id, "err", err)
}
