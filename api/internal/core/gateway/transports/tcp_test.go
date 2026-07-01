package transports

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTCP_ValidAddress(t *testing.T) {
	l, err := NewTCP("tcp://127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	_, ok := l.(*net.TCPListener)
	assert.True(t, ok)
}

func TestNewTCP_InvalidAddress_ReturnsError(t *testing.T) {
	_, err := NewTCP("tcp://not-a-valid-addr:99999")
	assert.Error(t, err)
}

// TestIsNonLoopbackBind proves R17's bind classifier: wildcard/LAN/DNS addresses
// are flagged as network-exposed (warned), loopback addresses are not.
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
