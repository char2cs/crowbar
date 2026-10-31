package stream

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// DocSource serves the doc scope. Its snapshot is empty: the client reads the
// document once synced says the watch is armed. Each hold is admitted first,
// so a client that may not read the path never arms a watch on it.
type DocSource struct {
	admit func(ctx context.Context, path string) error
}

// NewDocSource builds the doc scope's source over admit.
func NewDocSource(admit func(ctx context.Context, path string) error) DocSource {
	return DocSource{admit: admit}
}

// Admit answers whether this client may hold path.
func (s DocSource) Admit(ctx context.Context, path string) error { return s.admit(ctx, path) }

// Attach emits nothing.
func (DocSource) Attach(context.Context, string, int64, func(stream.Frame)) error { return nil }
