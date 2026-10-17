package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// TestRun_ServeError_SurfacesNotHangs proves H16: when Serve fails (the listener
// is invalid / accept fails permanently), Run must return the error so the
// process exits non-zero and its supervisor restarts the daemon — rather than
// silently blocking forever on <-ctx.Done() while the desktop spins reconnecting
// to a socket that never accepts.
func TestRun_ServeError_SurfacesNotHangs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close()) // closed listener -> Serve returns immediately

	hooks, err := hookserver.Listen(context.Background(), http.NewServeMux())
	require.NoError(t, err)
	t.Cleanup(func() { _ = hooks.Close() })
	server := &http.Server{Handler: http.NewServeMux()}
	c := &Container{
		server:   server,
		listener: ln,
		serveErr: make(chan error, 2),
		stack:    &stack{hooks: hooks},
	}
	c.serve(func() error { return server.Serve(ln) })

	// Block on Run's own return — the real signal. A 2-second deadline here would
	// be a guess about how fast a loaded machine schedules a goroutine; if Run
	// ever regresses to hanging on <-ctx.Done(), this receive never completes and
	// `go test -timeout` is the backstop that says so.
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	require.Error(t, <-done, "Run must surface the Serve failure instead of swallowing it")
}

// A halted boot whose error carries a failure besides the halt, such as closing
// what it opened, warns with all of it; the halt alone is serve's to announce.
func TestWarnHaltedBoot_WarnsOnlyOfFailuresBesidesTheHalt(t *testing.T) {
	halt := &migrate.Halt{Reason: migrate.HaltStepFailed, Message: "step 1 of 5 failed"}
	closeFailure := errors.New("injected close failure")
	warnings := testutil.CaptureLog(t, testutil.AtLeast(slog.LevelWarn))

	warnHaltedBoot(errors.Join(fmt.Errorf("internal: app: %w", errors.Join(fmt.Errorf("app: migrate: %w", halt)))))
	require.Empty(t, warnings, "the halt alone")
	warnHaltedBoot(errors.Join(fmt.Errorf("internal: app: %w", halt), closeFailure))

	require.Len(t, warnings, 1)
	logged, ok := testutil.Attrs(<-warnings)["err"].Any().(error)
	require.True(t, ok, "the warning carries the boot's error")
	require.ErrorIs(t, logged, closeFailure)
	require.ErrorIs(t, logged, halt)
}
