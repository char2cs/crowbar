package testutil

import (
	"context"
	"log/slog"
	"testing"
)

// logHook hands each record match accepts to f and drops the rest.
type logHook struct {
	match func(slog.Record) bool
	f     func(slog.Record)
}

// OnLog is a handler that calls f with each record whose message is msg, on
// the goroutine that logs it, so a test can act at that exact point.
func OnLog(
	msg string,
	f func(slog.Record),
) slog.Handler {
	return logHook{match: Message(msg), f: f}
}

// Message matches the records logged with message msg.
func Message(
	msg string,
) func(slog.Record) bool {
	return func(r slog.Record) bool { return r.Message == msg }
}

// AtLeast matches the records logged at level or above.
func AtLeast(
	level slog.Level,
) func(slog.Record) bool {
	return func(r slog.Record) bool { return r.Level >= level }
}

// CaptureLog sends the default logger's records that match accepts to the
// channel it returns, until the test ends. It keeps the first 16 and drops the
// rest, so a logger never blocks on a test that stopped reading.
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
