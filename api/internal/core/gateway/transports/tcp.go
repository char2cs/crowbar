package transports

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
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
		return listenPlaintext(addr)
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

// listenPlaintext binds the loopback dev listener and says loudly what it is:
// no request on it is authenticated, so anything that relays remote traffic
// onto it (a reverse proxy, an ssh -L, a published port) hands that traffic
// the whole API.
func listenPlaintext(addr string) (net.Listener, error) {
	ln, err := listenTCP(addr)
	if err != nil {
		return nil, err
	}
	slog.Warn("gateway: plaintext TCP is unauthenticated and for local development only; "+
		"never reverse-proxy or port-forward it — bind a non-loopback address to serve other machines over TLS",
		"component", "gateway", "addr", ln.Addr().String())
	return ln, nil
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

// IsNetworkExposed reports whether a tcp:// host binds beyond loopback — the
// bind NewTCP terminates TLS for.
func IsNetworkExposed(host string) bool {
	return isNonLoopbackBind(strings.TrimPrefix(host, "tcp://"))
}

// isNonLoopbackBind reports whether addr binds beyond loopback. Only a literal
// loopback IP (127.0.0.0/8, ::1) or exactly "localhost" stays local: an empty
// or wildcard host binds every interface, and any other DNS name resolves
// wherever its owner points it, so it is treated as exposed.
func isNonLoopbackBind(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port (or malformed): treat the whole string as the host.
		h = addr
	}
	if h == "localhost" {
		return false
	}
	if ip, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return !ip.IsLoopback()
	}
	return true
}
