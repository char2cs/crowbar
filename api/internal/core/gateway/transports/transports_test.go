package transports_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func TestNewTCP_HappyPath(t *testing.T) {
	l, err := transports.NewTCP("tcp://127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer l.Close()
	if _, ok := l.(*net.TCPListener); !ok {
		t.Fatalf("expected TCPListener, got %T", l)
	}
}

func TestNewTCP_InvalidAddress(t *testing.T) {
	_, err := transports.NewTCP("tcp://invalid-host-name-that-does-not-exist:99999")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestNewSocket_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	l, err := transports.NewSocket("unix://" + sockPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer l.Close()
	if l.Addr().Network() != "unix" {
		t.Fatalf("expected unix network, got %q", l.Addr().Network())
	}
}

func TestNewSocket_EmptyPathUsesDefault(t *testing.T) {
	l, err := transports.NewSocket("unix://")
	if err != nil {
		t.Fatalf("expected no error for default socket path, got: %v", err)
	}
	defer l.Close()
	if l.Addr().Network() != "unix" {
		t.Fatalf("expected unix network, got %q", l.Addr().Network())
	}
}

func TestNewSocket_HomeDirError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("USERPROFILE", "")
	_, err := transports.NewSocket("")
	if err == nil {
		t.Skip("os.UserHomeDir succeeded despite empty HOME — environment does not trigger the error path")
	}
}

func TestNewSocket_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a file at the location where MkdirAll would create a dir.
	crowbarPath := filepath.Join(dir, ".crowbar")
	if err := os.WriteFile(crowbarPath, []byte("x"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("HOME", dir)
	_, err := transports.NewSocket("")
	if err == nil {
		t.Fatal("expected error when .crowbar is a file, got nil")
	}
}
