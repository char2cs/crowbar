package transports

import (
	"crypto/tls"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

func TestNewTCP_ValidAddress(t *testing.T) {
	l, err := NewTCP("tcp://127.0.0.1:0", nil)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	_, ok := l.(*net.TCPListener)
	assert.True(t, ok)
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

// TestNewTCP_NonLoopback_ServerTLSOnly_NoClientCertRequired proves the TLS
// config NewTCP wraps a non-loopback bind with never requests a client
// certificate. It dials tlsListen directly over a loopback listener — the
// same wrapping NewTCP applies to a real non-loopback bind — rather than
// binding 0.0.0.0 itself, which on macOS can block on an "accept incoming
// connections" prompt with no one there to answer it. A tls.Listener only
// completes a handshake once something calls Accept on it — production gets
// that from http.Server's own accept loop, so the test drives it with one of
// its own, in a goroutine so it runs concurrently with the blocking client
// dial below.
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

	// Dial without presenting any client certificate — must succeed the
	// handshake (server-TLS-only, not mTLS).
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

// TestIsNonLoopbackBind proves R17's bind classifier: wildcard/LAN/DNS addresses
// are flagged as network-exposed (requiring a TLS identity), loopback addresses
// are not.
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
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			if got := isNonLoopbackBind(tc.addr); got != tc.want {
				t.Fatalf("isNonLoopbackBind(%q) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}
