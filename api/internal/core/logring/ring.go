package logring

import "log/slog"

type Ring interface {
	Wrap(next slog.Handler) slog.Handler

	Stream(since uint64, level slog.Level) Stream
}
