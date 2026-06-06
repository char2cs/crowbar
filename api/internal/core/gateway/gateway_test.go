package gateway_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/gateway"
)

func TestGatewayTCP(t *testing.T) {
	l, err := gateway.New("tcp://127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer l.Close()
	if _, ok := l.(*net.TCPListener); !ok {
		t.Fatalf("expected TCPListener, got %T", l)
	}
}

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080")
	if err == nil {
		t.Fatal("expected error for unknown scheme, got nil")
	}
}

func TestGatewayUnix_ExplicitPath(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	l, err := gateway.New("unix://" + sockPath)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
}

func TestGatewayUnix_DefaultPath(t *testing.T) {
	home, err := os.MkdirTemp("/tmp", "cb")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	l, err := gateway.New("unix://")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	assert.NotNil(t, l)
}
