package internal

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

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

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	select {
	case runErr := <-done:
		require.Error(t, runErr, "Run must surface the Serve failure instead of swallowing it")
	case <-time.After(2 * time.Second):
		t.Fatal("Run hung on a Serve failure instead of returning the error")
	}
}
