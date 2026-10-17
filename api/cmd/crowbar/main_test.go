package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestCLIHelp(
	t *testing.T,
) {
	cmd := exec.Command("go", "run", ".", "--help")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("output: %s", out)
	}
	if len(out) == 0 {
		t.Fatal("expected help output, got nothing")
	}
}

// serveSocket is a unix socket host of the test's own, on a short path.
func serveSocket(
	t *testing.T,
) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	return "unix://" + filepath.Join(dir, "s.sock")
}

// A signal during the migration interrupts it; the next boot resumes, so the
// process exits cleanly rather than as a failed start.
func TestServe_ACancelledBootExitsCleanly(t *testing.T) {
	h := testutil.FixtureHome(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := serve(ctx, serveSocket(t), nil, internal.WithHomeDir(h.Root()))

	assert.NoError(t, err)
}

// haltLog signals each "daemon: halted" record it is handed.
type haltLog chan slog.Record

func (haltLog) Enabled(context.Context, slog.Level) bool { return true }

func (l haltLog) Handle(
	_ context.Context,
	r slog.Record,
) error {
	if r.Message == "daemon: halted" {
		l <- r
	}
	return nil
}

func (l haltLog) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l haltLog) WithGroup(string) slog.Handler { return l }

// A halted daemon says so in its log and keeps serving its health route until
// it is stopped, which is a clean exit: a supervisor must not respawn it.
func TestServe_AHaltedDaemonLogsWhyAndServesUntilStopped(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")
	halted := make(haltLog, 1)
	prev := slog.Default()
	slog.SetDefault(slog.New(halted))
	t.Cleanup(func() { slog.SetDefault(prev) })
	ctx, cancel := context.WithCancel(t.Context())
	host := serveSocket(t)
	done := make(chan error, 1)

	go func() { done <- serve(ctx, host, nil, internal.WithHomeDir(h.Root())) }()
	record := <-halted
	cancel()

	require.NoError(t, <-done)
	attrs := map[string]string{}
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	assert.Equal(t, slog.LevelError, record.Level)
	assert.Equal(t, "layout_newer", attrs["reason"])
	assert.Contains(t, attrs["message"], "this home is at layout 9")
}
