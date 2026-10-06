package handlers

import (
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/logring"
)

type fakeRing struct {
	stream *fakeStream
}

func (r *fakeRing) Wrap(
	next slog.Handler,
) slog.Handler {
	return next
}

func (r *fakeRing) Stream(
	_ uint64,
	_ slog.Level,
) logring.Stream {
	return r.stream
}
