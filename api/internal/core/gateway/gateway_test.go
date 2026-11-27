package gateway_test

import (
	"net"
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

func TestGatewayTCP(t *testing.T) {
	ls, err := gateway.New("tcp://127.0.0.1:0", "")
	require.NoError(t, err)
	closeAll(t, ls)
	require.Len(t, ls, 1)
	_, ok := ls[0].(*net.TCPListener)
	assert.Truef(t, ok, "expected TCPListener, got %T", ls[0])
}

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080", "")
	assert.Error(t, err)
}

func TestGatewayUnix_ExplicitPath(t *testing.T) {
	sockPath := testutil.ShortSocket(t)
	ls, err := gateway.New("unix://"+sockPath, "")
	require.NoError(t, err)
	closeAll(t, ls)
	assert.Len(t, ls, 1)
}

func TestGatewayUnix_BarePathBindsTheGivenSocket(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir())
	sock := testutil.ShortSocket(t)

	ls, err := gateway.New("unix://", sock)

	require.NoError(t, err)
	closeAll(t, ls)
	assert.FileExists(t, sock)
}

func TestRegression_New_NetworkExposedHostIsTheDeviceListenersNotAGatewayBind(t *testing.T) {
	for _, host := range []string{"tcp://0.0.0.0:0", "tcp://:3737", "tcp://192.168.1.50:3737"} {
		ls, err := gateway.New(host, testutil.ShortSocket(t))
		require.Errorf(t, err, "%s", host)
		assert.Empty(t, ls)
		assert.True(t, gateway.Exposed(host), host)
	}
	for _, host := range []string{"unix://", "tcp://127.0.0.1:3737", "tcp://localhost:3737"} {
		assert.Falsef(t, gateway.Exposed(host), "%s", host)
	}
}
