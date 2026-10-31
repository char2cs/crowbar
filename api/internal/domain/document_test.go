package domain_test

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Only a request over the unix socket is local: TCP, even loopback, and a
// context with no listener address are remote.
func TestTransportOf_OnlyTheUnixSocketIsLocal(t *testing.T) {
	at := func(addr net.Addr) context.Context {
		return context.WithValue(context.Background(), http.LocalAddrContextKey, addr)
	}
	assert.Equal(t, domain.TransportLocal, domain.TransportOf(at(&net.UnixAddr{Name: "/s", Net: "unix"})))
	assert.Equal(t, domain.TransportRemote, domain.TransportOf(at(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})))
	assert.Equal(t, domain.TransportRemote, domain.TransportOf(context.Background()))
}
