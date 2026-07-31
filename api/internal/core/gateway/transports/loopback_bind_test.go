package transports_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

// TestNewLoopback_BindsAnEphemeralLoopbackPort proves the default asks the OS for
// a port and that the caller can read the assigned one back off the listener —
// the mechanism that lets several checkouts run at once without colliding.
func TestNewLoopback_BindsAnEphemeralLoopbackPort(t *testing.T) {
	l, err := transports.NewLoopback("127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)
	assert.True(t, addr.IP.IsLoopback())
	assert.NotZero(t, addr.Port, "the OS must have assigned a concrete port")

	conn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()
}

func TestNewLoopback_AcceptsAnOptionalTCPScheme(t *testing.T) {
	l, err := transports.NewLoopback("tcp://127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	assert.NotNil(t, l)
}

func TestNewLoopback_AcceptsIPv6Loopback(t *testing.T) {
	l, err := transports.NewLoopback("[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback on this host")
	}
	t.Cleanup(func() { _ = l.Close() })
	assert.NotNil(t, l)
}

// TestNewLoopback_RefusesNonLoopback is the security boundary: a wildcard, a LAN
// address and any hostname (which the OS resolves, and which /etc/hosts or a DNS
// search domain can point off-box) are refused outright. Unlike NewTCP this does
// not warn-and-continue — the listener carries the daemon's full authority.
func TestNewLoopback_RefusesNonLoopback(t *testing.T) {
	refused := map[string]string{
		"ipv4 wildcard":       "0.0.0.0:3737",
		"empty host":          ":3737",
		"ipv6 wildcard":       "[::]:3737",
		"lan address":         "192.168.1.50:3737",
		"private class a":     "10.0.0.5:3737",
		"public address":      "8.8.8.8:3737",
		"hostname":            "example.com:3737",
		"localhost hostname":  "localhost:3737",
		"scheme and wildcard": "tcp://0.0.0.0:3737",
		"no port":             "127.0.0.1",
		"empty":               "",
	}
	for name, addr := range refused {
		t.Run(name, func(t *testing.T) {
			l, err := transports.NewLoopback(addr)
			if l != nil {
				_ = l.Close()
			}
			require.Error(t, err, "%q must be refused", addr)
			assert.ErrorIs(t, err, transports.ErrNonLoopbackBind)
		})
	}
}

// TestNewLoopback_PortInUse_ReturnsError proves an explicit port that is already
// taken surfaces as a startup error rather than a silent no-listener.
func TestNewLoopback_PortInUse_ReturnsError(t *testing.T) {
	held, err := transports.NewLoopback("127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })

	l, err := transports.NewLoopback(held.Addr().String())
	if l != nil {
		_ = l.Close()
	}
	require.Error(t, err)
	assert.NotErrorIs(t, err, transports.ErrNonLoopbackBind, "a busy port is a bind failure, not a policy refusal")
}
