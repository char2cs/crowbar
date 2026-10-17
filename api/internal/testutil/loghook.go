package testutil

import (
	"context"
	"log/slog"
)

// logHook hands each record logged with message msg to f and drops the rest.
type logHook struct {
	msg string
	f   func(slog.Record)
}

// OnLog is a handler that calls f with each record whose message is msg, on
// the goroutine that logs it, so a test can act at that exact point.
func OnLog(
	msg string,
	f func(slog.Record),
) slog.Handler {
	return logHook{msg: msg, f: f}
}

func (logHook) Enabled(context.Context, slog.Level) bool { return true }

func (h logHook) Handle(
	_ context.Context,
	r slog.Record,
) error {
	if r.Message == h.msg {
		h.f(r)
	}
	return nil
}

func (h logHook) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h logHook) WithGroup(string) slog.Handler { return h }

// Attrs is a record's attributes by key.
func Attrs(
	r slog.Record,
) map[string]slog.Value {
	attrs := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	return attrs
}
