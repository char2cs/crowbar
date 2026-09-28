package transports

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

// NewTCP binds a TCP listener. A loopback bind stays plaintext — the dev
// path — regardless of id. A non-loopback bind terminates TLS using id's
// server identity when supplied, and is refused outright when it is not; no
// client certificate is ever requested (server-TLS-only, not mTLS — auth is
// the bearer token carried in the request, not the connection).
func NewTCP(
	host string,
	id *tlsid.Identity,
) (net.Listener, error) {
	addr := strings.TrimPrefix(host, "tcp://")
	if !isNonLoopbackBind(addr) {
		// Loopback is the dev path and stays plaintext: make dev-api and
		// crowbar-seed both dial it over http.
		return listenTCP(addr)
	}
	if id == nil {
		return nil, fmt.Errorf(
			"gateway: refusing to bind %s without a TLS identity — the API would be "+
				"reachable and unauthenticated", addr)
	}

	ln, err := listenTCP(addr)
	if err != nil {
		return nil, err
	}
	return tlsListen(ln, id), nil
}

// listenTCP binds addr. There is no caller-supplied context to thread
// through here — NewTCP runs once at daemon startup — so it uses Background.
func listenTCP(addr string) (net.Listener, error) {
	return (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
}

// tlsListen wraps ln to terminate TLS using id's server identity, requesting
// no client certificate (server-TLS-only, not mTLS).
func tlsListen(ln net.Listener, id *tlsid.Identity) net.Listener {
	return tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{id.Certificate},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS13,
	})
}

// isNonLoopbackBind reports whether addr binds beyond loopback. An empty or
// wildcard host (":3737", "0.0.0.0:3737", "[::]:3737") binds all interfaces; a
// concrete loopback host (127.0.0.0/8, ::1, localhost) does not.
func isNonLoopbackBind(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port (or malformed): treat the whole string as the host.
		h = addr
	}
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		return true
	case "localhost", "::1", "[::1]":
		return false
	}
	if strings.HasPrefix(h, "127.") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return !ip.IsLoopback()
	}
	// A non-IP, non-localhost host (a DNS name): conservatively treat as exposed.
	return true
}
