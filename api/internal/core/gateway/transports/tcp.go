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

func NewTCP(
	host string,
	id *tlsid.Identity,
) (net.Listener, error) {
	addr := strings.TrimPrefix(host, "tcp://")
	if !isNonLoopbackBind(addr) {
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

func listenTCP(addr string) (net.Listener, error) {
	return (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
}

func tlsListen(ln net.Listener, id *tlsid.Identity) net.Listener {
	return tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{id.Certificate},
		ClientAuth:   tls.NoClientCert,
		MinVersion:   tls.VersionTLS13,
	})
}

func IsNetworkExposed(host string) bool {
	return isNonLoopbackBind(strings.TrimPrefix(host, "tcp://"))
}

func isNonLoopbackBind(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
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
