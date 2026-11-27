package internal

import "log/slog"

func WithMigrateLog(
	l *slog.Logger,
) Option {
	return func(o *rootOpts) {
		o.migrateLog = l
	}
}
