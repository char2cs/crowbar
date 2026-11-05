package logring

import "log/slog"

// Ring is a fixed-size, concurrency-safe store of the daemon's latest log
// records that can be replayed and followed.
type Ring interface {
	// Wrap returns a slog.Handler that stores every record next handles and then passes
	// it on, so the ring sees exactly the records the daemon's own log level enables.
	Wrap(next slog.Handler) slog.Handler

	// Stream opens a Stream of the records at level or above, replaying after since (the
	// last 500 when zero; a since past the newest Seq is a restart, flagged by Reset). The
	// replay and live channel are cut at one instant, so nothing is missed or repeated.
	Stream(since uint64, level slog.Level) Stream
}
