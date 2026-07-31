package transports

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrNonLoopbackBind is returned when the auxiliary TCP listener is asked to
// bind an address that is reachable from outside this machine. Unlike NewTCP —
// which honours an operator's explicit non-loopback bind and merely warns — the
// loopback listener REFUSES. It exists to be reached by the local native client
// and the local webview only, and it carries the daemon's full authority (live
// shells, file read/write, git), so "the operator probably meant it" is not a
// defensible default here.
var ErrNonLoopbackBind = errors.New("gateway: loopback: refusing a non-loopback bind address")

// NewLoopback binds the daemon's auxiliary TCP listener on an explicit loopback
// address. addr is a "host:port" pair (an optional "tcp://" prefix is accepted);
// a zero port asks the OS for an ephemeral one, which is the default because a
// fixed port collides across the several worktrees this repo runs at once — read
// the real port back from the returned listener's Addr.
//
// The address is validated BEFORE the bind, and the rule is deliberately strict:
// the host must be a literal loopback IP. A wildcard host ("", "0.0.0.0", "::")
// binds every interface; a hostname — including "localhost" — is resolved by the
// OS and can be pointed off-box by /etc/hosts or a DNS search domain, so it is
// not proof of anything. Only an IP literal that reports IsLoopback is accepted.
func NewLoopback(
	addr string,
) (net.Listener, error) {
	bind := strings.TrimPrefix(addr, "tcp://")
	if err := requireLoopback(bind); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, fmt.Errorf("gateway: loopback: listen %s: %w", bind, err)
	}
	return ln, nil
}

func requireLoopback(
	bind string,
) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("%w: %q is not a host:port address: %w", ErrNonLoopbackBind, bind, err)
	}
	if host == "" {
		return fmt.Errorf(
			"%w: %q binds every interface; use an explicit loopback IP such as 127.0.0.1:0",
			ErrNonLoopbackBind, bind,
		)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf(
			"%w: %q is a hostname and can resolve off-box; use an explicit loopback IP such as 127.0.0.1:0",
			ErrNonLoopbackBind, bind,
		)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf(
			"%w: %q is reachable from the network; use an explicit loopback IP such as 127.0.0.1:0",
			ErrNonLoopbackBind, bind,
		)
	}
	return nil
}
