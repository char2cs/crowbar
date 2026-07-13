package internal

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
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

	c := &Container{
		server:   &http.Server{Handler: http.NewServeMux()},
		listener: ln,
	}

	// Block on Run's own return — the real signal. A 2-second deadline here would
	// be a guess about how fast a loaded machine schedules a goroutine; if Run
	// ever regresses to hanging on <-ctx.Done(), this receive never completes and
	// `go test -timeout` is the backstop that says so.
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	require.Error(t, <-done, "Run must surface the Serve failure instead of swallowing it")
}
