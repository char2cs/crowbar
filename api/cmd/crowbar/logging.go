package main

import (
	"log/slog"
	"os"

	"github.com/char2cs/crowbar/api/internal/core/logring"
)

func installLogRing() logring.Ring {
	ring := logring.New()
	next := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(ring.Wrap(next)))
	return ring
}
