package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os/exec"
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

// healthStatus is the status the daemon on sock reports on its health route.
func healthStatus(
	t *testing.T,
	sock string,
) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://crowbar/v0/health", http.NoBody)
	require.NoError(t, err)
	resp, err := testutil.UnixClient(t, sock).Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var health struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	return health.Data.Status
}

// A signal during the migration interrupts it; the next boot resumes, so the
// process exits cleanly rather than as a failed start, and logs what New said.
func TestServe_ACancelledBootExitsCleanly(t *testing.T) {
	h := testutil.FixtureHome(t)
	interrupted := testutil.CaptureLog(t, testutil.Message("daemon: boot interrupted"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := serve(ctx, "unix://"+testutil.ShortSocket(t), nil, internal.WithHomeDir(h.Root()))

	require.NoError(t, err)
	var record slog.Record
	select {
	case record = <-interrupted:
	default:
		t.Fatal("serve said nothing of the boot it gave up")
	}
	logged, ok := testutil.Attrs(record)["err"].Any().(error)
	require.True(t, ok, "the record carries New's error")
	require.ErrorIs(t, logged, context.Canceled)
	assert.Contains(t, logged.Error(), "internal: migrate: ")
}

// A halted daemon says so in one error line and keeps serving its health route
// until it is stopped, which is a clean exit: a supervisor must not respawn it.
func TestServe_AHaltedDaemonLogsWhyAndServesUntilStopped(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")
	errorLines := testutil.CaptureLog(t, testutil.AtLeast(slog.LevelError))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sock := testutil.ShortSocket(t)
	done := make(chan error, 1)

	go func() { done <- serve(ctx, "unix://"+sock, nil, internal.WithHomeDir(h.Root())) }()
	var record slog.Record
	select {
	case record = <-errorLines:
	case err := <-done:
		t.Fatalf("serve returned before it logged the halt: %v", err)
	}
	status := healthStatus(t, sock)
	select {
	case err := <-done:
		t.Fatalf("serve returned before it was stopped: %v", err)
	default:
	}
	cancel()

	require.NoError(t, <-done)
	assert.Equal(t, "halted", status)
	assert.Equal(t, "daemon: halted", record.Message, "the first error line names the halt")
	assert.Empty(t, errorLines, "one halt, one error line")
	attrs := testutil.Attrs(record)
	assert.Equal(t, slog.LevelError, record.Level)
	assert.Equal(t, "layout_newer", attrs["reason"].String())
	assert.Contains(t, attrs["message"].String(), "this home is at layout 9")
}
