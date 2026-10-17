package internal

import "log/slog"

// WithMigrateLog sends the migration session's log to l.
func WithMigrateLog(
	l *slog.Logger,
) Option {
	return func(o *rootOpts) {
		o.migrateLog = l
	}
}
