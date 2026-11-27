package testutil

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func ShortSocket(
	tb testing.TB,
) string {
	tb.Helper()
	dir, err := os.MkdirTemp("", "cb")
	if err != nil {
		tb.Fatalf("short socket: %v", err)
	}
	tb.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			tb.Errorf("short socket: remove %s: %v", dir, err)
		}
	})
	return filepath.Join(dir, "s.sock")
}

func UnixClient(
	tb testing.TB,
	sock string,
) *http.Client {
	tb.Helper()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}
	tb.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport}
}
