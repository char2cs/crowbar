package ipc_test

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

// TestNewClient_BadHost_ReturnsError forces transports.SocketPath's default
// derivation to fail: CROWBAR_HOME is a relative path, so NewClient must wrap
// and return the resolver's error rather than panic.
func TestNewClient_BadHost_ReturnsError(t *testing.T) {
	t.Setenv(home.EnvVar, "relative/home")

	_, err := ipc.NewClient("unix://")

	require.ErrorContains(t, err, "must be an absolute path")
}

// TestClient_Get_DialFailure_ReturnsError points the client at a socket path
// with no listener behind it: Get must surface a transport error, not panic.
func TestClient_Get_DialFailure_ReturnsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nope.sock")
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	status, body, err := c.Get(context.Background(), "/v0/x")
	require.Error(t, err)
	require.Zero(t, status)
	require.Nil(t, body)
}

// TestClient_PostJSON_DialFailure_ReturnsError is the PostJSON analogue of the
// Get dial-failure case above.
func TestClient_PostJSON_DialFailure_ReturnsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nope.sock")
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	status, body, err := c.PostJSON(context.Background(), "/v0/x", map[string]string{"a": "b"})
	require.Error(t, err)
	require.Zero(t, status)
	require.Nil(t, body)
}

// TestClient_Get_RequestCreationFailure_ReturnsError covers the request path
// containing an invalid control character, which url.Parse rejects — the
// http.NewRequestWithContext error branch, never exercised via a well-formed
// path.
func TestClient_Get_RequestCreationFailure_ReturnsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "unused.sock")
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	_, _, err = c.Get(context.Background(), "/bad\npath")
	require.Error(t, err)
}

// TestClient_PostJSON_RequestCreationFailure_ReturnsError is the PostJSON
// analogue of the Get request-creation-failure case above.
func TestClient_PostJSON_RequestCreationFailure_ReturnsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "unused.sock")
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	_, _, err = c.PostJSON(context.Background(), "/bad\npath", map[string]string{"a": "b"})
	require.Error(t, err)
}

// TestClient_PostJSON_MarshalFailure_ReturnsError covers json.Marshal's error
// branch with a body type that can never be encoded.
func TestClient_PostJSON_MarshalFailure_ReturnsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "unused.sock")
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	_, _, err = c.PostJSON(context.Background(), "/v0/x", make(chan int))
	require.Error(t, err)
}

// TestClient_ATruncatedBodyIsReturned has the daemon promise more bytes than it
// sends: a reply cut short is a transport failure, never a short answer.
func TestClient_ATruncatedBodyIsReturned(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "t.sock")
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		if _, werr := w.Write([]byte(`{"su`)); werr != nil {
			t.Error(werr)
		}
	})}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	t.Cleanup(func() {
		require.NoError(t, srv.Close())
		require.ErrorIs(t, <-served, http.ErrServerClosed)
	})
	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	_, _, err = c.Get(context.Background(), "/v0/x")
	require.Error(t, err)
	_, _, err = c.PostJSON(context.Background(), "/v0/x", map[string]string{"a": "b"})
	require.Error(t, err)
}
