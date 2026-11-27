package transports

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

func TestNewTCP_ValidAddress(t *testing.T) {
	l, err := NewTCP("tcp://127.0.0.1:0", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	_, ok := l.(*net.TCPListener)
	assert.True(t, ok)
}

func TestNewTCP_Loopback_WarnsItIsUnauthenticated(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(restore) })

	l, err := NewTCP("tcp://127.0.0.1:0", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	assert.Contains(t, logged.String(), "level=WARN")
	assert.Contains(t, logged.String(), "unauthenticated")
	assert.Contains(t, logged.String(), "port-forward")
}

func TestNewTCP_InvalidAddress_ReturnsError(t *testing.T) {
	id, err := tlsid.Load(t.TempDir())
	require.NoError(t, err)
	_, err = NewTCP("tcp://not-a-valid-addr:99999", id)
	assert.Error(t, err)
}

func TestNewTCP_NonLoopback_NoIdentity_Refused(t *testing.T) {
	_, err := NewTCP("tcp://0.0.0.0:0", nil)
	assert.ErrorContains(t, err, "TLS identity")
}

func TestNewTCP_NonLoopback_ServerTLSOnly_NoClientCertRequired(t *testing.T) {
	dir := t.TempDir()
	id, err := tlsid.Load(dir)
	if err != nil {
		t.Fatalf("tlsid.Load: %v", err)
	}
	raw, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	ln := tlsListen(raw, id)
	defer func() { _ = ln.Close() }()

	handshaked := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			handshaked <- err
			return
		}
		defer func() { _ = conn.Close() }()
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			handshaked <- fmt.Errorf("accepted connection is %T, not *tls.Conn", conn)
			return
		}
		handshaked <- tlsConn.HandshakeContext(t.Context())
	}()

	dialer := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}
	conn, err := dialer.DialContext(t.Context(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("expected a client with no cert to complete the TLS handshake, got: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := <-handshaked; err != nil {
		t.Fatalf("server-side handshake failed: %v", err)
	}
}

func acceptOne(
	t *testing.T,
	ln net.Listener,
) <-chan error {
	t.Helper()
	handshaked := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			handshaked <- err
			return
		}
		defer func() { _ = conn.Close() }()
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			handshaked <- fmt.Errorf("accepted connection is %T, not *tls.Conn", conn)
			return
		}
		handshaked <- tlsConn.HandshakeContext(t.Context())
	}()
	return handshaked
}

func TestTLSListen_TerminatesOnlyTLS13(t *testing.T) {
	id, err := tlsid.Load(t.TempDir())
	require.NoError(t, err)
	raw, err := listenTCP("127.0.0.1:0")
	require.NoError(t, err)
	ln := tlsListen(raw, id)
	t.Cleanup(func() { _ = ln.Close() })

	handshaked := acceptOne(t, ln)
	dialer := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}
	conn, err := dialer.DialContext(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err, "a TLS client must complete the handshake")
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, <-handshaked)
	tlsConn, ok := conn.(*tls.Conn)
	require.True(t, ok)
	assert.Equal(t, uint16(tls.VersionTLS13), tlsConn.ConnectionState().Version)

	handshaked = acceptOne(t, ln)
	legacy := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}}
	_, err = legacy.DialContext(t.Context(), "tcp", ln.Addr().String())
	require.Error(t, err, "a TLS 1.2 client must be refused")
	assert.Error(t, <-handshaked, "the server must refuse the TLS 1.2 handshake")
}

func TestIsNonLoopbackBind(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"0.0.0.0:3737", true},
		{":3737", true},
		{"[::]:3737", true},
		{"192.168.1.50:3737", true},
		{"10.0.0.5:3737", true},
		{"example.com:3737", true},
		{"127.0.0.1:3737", false},
		{"127.0.0.2:3737", false},
		{"localhost:3737", false},
		{"[::1]:3737", false},
		{"::1", false},
		{"localhost.:3737", true},
		{"127.example.com:3737", true},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			if got := isNonLoopbackBind(tc.addr); got != tc.want {
				t.Fatalf("isNonLoopbackBind(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestRegression_NewTCP_DNSNameShapedLikeLoopbackIsExposed(t *testing.T) {
	const host = "tcp://127.example.com:3737"
	assert.True(t, IsNetworkExposed(host))
	_, err := NewTCP(host, nil)
	assert.ErrorContains(t, err, "TLS identity", "an exposed bind without an identity must be refused, not bound plaintext")
}
