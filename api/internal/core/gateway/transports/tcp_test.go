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
