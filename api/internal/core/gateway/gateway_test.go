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

func TestGatewayTCP(t *testing.T) {
	l, err := gateway.New("tcp://127.0.0.1:0", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer l.Close()
	if _, ok := l.(*net.TCPListener); !ok {
		t.Fatalf("expected TCPListener, got %T", l)
	}
}

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080", "")
	if err == nil {
		t.Fatal("expected error for unknown scheme, got nil")
	}
}

func TestGatewayUnix_ExplicitPath(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	l, err := gateway.New("unix://"+sockPath, "")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
}

// A bare "unix://" binds the socket the caller resolved, never one New looks
// up from the environment on its own.
func TestGatewayUnix_BarePathBindsTheGivenSocket(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir())
	sock := testutil.ShortSocket(t)

	l, err := gateway.New("unix://", sock)

	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.FileExists(t, sock)
}
