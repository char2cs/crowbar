package gateway_test

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func closeAll(t *testing.T, ls []net.Listener) {
	t.Helper()
	t.Cleanup(func() {
		for _, l := range ls {
			_ = l.Close()
		}
	})
}

// TestGatewayTCP proves a loopback tcp:// bind is the dev path: one plaintext
// listener and no unix socket beside it.
func TestGatewayTCP(t *testing.T) {
	ls, err := gateway.New("tcp://127.0.0.1:0", "", nil)
	require.NoError(t, err)
	closeAll(t, ls)
	require.Len(t, ls, 1)
	_, ok := ls[0].(*net.TCPListener)
	assert.Truef(t, ok, "expected TCPListener, got %T", ls[0])
}

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080", "", nil)
	assert.Error(t, err)
}

func TestGatewayUnix_ExplicitPath(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	ls, err := gateway.New("unix://"+sockPath, "", nil)
	require.NoError(t, err)
	closeAll(t, ls)
	assert.Len(t, ls, 1)
}

// A bare "unix://" binds the socket the caller resolved, never one New looks
// up from the environment on its own.
func TestGatewayUnix_BarePathBindsTheGivenSocket(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir())
	sock := testutil.ShortSocket(t)

	ls, err := gateway.New("unix://", sock, nil)

	require.NoError(t, err)
	closeAll(t, ls)
	assert.FileExists(t, sock)
}

// TestRegression_BindAddrs_NetworkExposedTCPAlsoBindsLocalSocket proves a
// daemon reachable over the network still binds the local unix socket that
// `crowbar pair` dials, so it can always be paired from its own host.
func TestRegression_BindAddrs_NetworkExposedTCPAlsoBindsLocalSocket(t *testing.T) {
	const sock = "/home/crowbar.sock"
	cases := []struct {
		host string
		want []string
	}{
		{"unix://", []string{"unix://" + sock}},
		{"unix:///tmp/x.sock", []string{"unix:///tmp/x.sock"}},
		{"tcp://127.0.0.1:3737", []string{"tcp://127.0.0.1:3737"}},
		{"tcp://localhost:3737", []string{"tcp://localhost:3737"}},
		{"tcp://0.0.0.0:3737", []string{"unix://" + sock, "tcp://0.0.0.0:3737"}},
		{"tcp://:3737", []string{"unix://" + sock, "tcp://:3737"}},
		{"tcp://192.168.1.50:3737", []string{"unix://" + sock, "tcp://192.168.1.50:3737"}},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, err := gateway.BindAddrs(tc.host, sock)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := gateway.BindAddrs("http://localhost:8080", sock)
	assert.Error(t, err)
}

// TestGateway_LaterBindFails_ReleasesEarlierListeners proves a half-bound
// gateway never leaks: the exposed TCP bind is refused (no TLS identity)
// after the unix socket is up, and New must close that socket again.
func TestGateway_LaterBindFails_ReleasesEarlierListeners(t *testing.T) {
	sock := testutil.ShortSocket(t)

	_, err := gateway.New("tcp://0.0.0.0:0", sock, nil)
	require.ErrorContains(t, err, "TLS identity")

	assert.NoFileExists(t, sock, "the unix socket must be released")
}
