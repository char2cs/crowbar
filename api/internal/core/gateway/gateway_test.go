package gateway_test

import (
	"net"
	"path/filepath"
	"testing"

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

func TestGatewayUnix(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "gw.sock")
	l, err := gateway.New("unix://" + sockPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer l.Close()
	if l.Addr().Network() != "unix" {
		t.Fatalf("expected unix network, got %q", l.Addr().Network())
	}
}

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080")
	if err == nil {
		t.Fatal("expected error for unknown scheme, got nil")
	}
}

func TestGatewayEmptyScheme(t *testing.T) {
	_, err := gateway.New("localhost:8080")
	if err == nil {
		t.Fatal("expected error for no scheme, got nil")
	}
}
