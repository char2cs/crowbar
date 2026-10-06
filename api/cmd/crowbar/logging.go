package main

import (
	"log/slog"
	"os"

	"github.com/char2cs/crowbar/api/internal/core/logring"
)

// installLogRing makes the daemon's slog default handler record into a ring
// (served by the console log stream) while still writing every Info+ line to
// stderr, which is daemon.log. The stdlib log package follows the default.
func installLogRing() logring.Ring {
	ring := logring.New()
	next := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(ring.Wrap(next)))
	return ring
}
