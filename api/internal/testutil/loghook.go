package testutil

import (
	"context"
	"log/slog"
	"testing"
)

type logHook struct {
	match func(slog.Record) bool
	f     func(slog.Record)
}

func OnLog(
	msg string,
	f func(slog.Record),
) slog.Handler {
	return logHook{match: Message(msg), f: f}
}

func Message(
	msg string,
) func(slog.Record) bool {
	return func(r slog.Record) bool { return r.Message == msg }
}

func AtLeast(
	level slog.Level,
) func(slog.Record) bool {
	return func(r slog.Record) bool { return r.Level >= level }
}

func CaptureLog(
	tb testing.TB,
	match func(slog.Record) bool,
) <-chan slog.Record {
	tb.Helper()
	records := make(chan slog.Record, 16)
	prev := slog.Default()
	slog.SetDefault(slog.New(logHook{match: match, f: func(r slog.Record) {
		select {
		case records <- r:
		default:
		}
	}}))
	tb.Cleanup(func() { slog.SetDefault(prev) })
	return records
}

func (logHook) Enabled(context.Context, slog.Level) bool { return true }

func (h logHook) Handle(
	_ context.Context,
	r slog.Record,
) error {
	if h.match(r) {
		h.f(r)
	}
	return nil
}

func (h logHook) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h logHook) WithGroup(string) slog.Handler { return h }

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
