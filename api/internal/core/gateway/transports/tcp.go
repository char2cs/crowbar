package transports

import (
	"log/slog"
	"net"
	"strings"
)

// NewTCP binds a TCP listener for the dev/explicit transport (production uses the
// unix socket). It logs a loud warning when asked to bind a non-loopback address
// (0.0.0.0, an empty host, or a LAN IP): that exposes the unauthenticated daemon
// API — live shells, file read/write, git — to every host on the network. The
// bind is honoured (an operator may want it), but it is never silent.
func NewTCP(host string) (net.Listener, error) {
	addr := strings.TrimPrefix(host, "tcp://")
	if isNonLoopbackBind(addr) {
		slog.Warn("gateway: binding daemon to a non-loopback TCP address; "+
			"the unauthenticated API is reachable from the network — prefer unix:// or tcp://127.0.0.1:<port>",
			"addr", addr)
	}
	return net.Listen("tcp", addr)
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
