package stream

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type DocSource struct {
	admit func(ctx context.Context, path string) error
}

func NewDocSource(admit func(ctx context.Context, path string) error) DocSource {
	return DocSource{admit: admit}
}

func (s DocSource) Admit(ctx context.Context, path string) error { return s.admit(ctx, path) }

func (DocSource) Attach(context.Context, string, int64, func(stream.Frame)) error { return nil }
