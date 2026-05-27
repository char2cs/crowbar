package gateway_test

import (
	"net"
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

func TestGatewayUnknownScheme(t *testing.T) {
	_, err := gateway.New("http://localhost:8080")
	if err == nil {
		t.Fatal("expected error for unknown scheme, got nil")
	}
}
