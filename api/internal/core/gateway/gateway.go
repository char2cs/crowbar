package gateway

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

// New binds every listener the daemon serves for host (see bindAddrs), closing
// any it already bound if a later one fails. socket is the socket of the home
// the caller holds, which a bare "unix://" binds, so the two never disagree.
func New(host, socket string, id *tlsid.Identity) ([]net.Listener, error) {
	addrs, err := bindAddrs(host, socket)
	if err != nil {
		return nil, err
	}
	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		ln, err := listen(addr, id)
		if err != nil {
			for _, bound := range listeners {
				err = errors.Join(err, bound.Close())
			}
			return nil, err
		}
		listeners = append(listeners, ln)
	}
	return listeners, nil
}

// Exposed reports whether host binds beyond loopback: the one bind that serves
// TLS, and so the only one that needs the daemon's TLS identity.
func Exposed(host string) bool {
	return strings.HasPrefix(host, "tcp://") && transports.IsNetworkExposed(host)
}

// bindAddrs lists the addresses New binds for host, in bind order. A
// network-exposed tcp:// host also binds the home's unix socket: `crowbar pair`
// only dials that socket, and pairing must work from the daemon's own host.
func bindAddrs(host, socket string) ([]string, error) {
	local := "unix://" + socket
	switch {
	case host == "unix://":
		return []string{local}, nil
	case strings.HasPrefix(host, "unix://"):
		return []string{host}, nil
	case Exposed(host):
		return []string{local, host}, nil
	case strings.HasPrefix(host, "tcp://"):
		return []string{host}, nil
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}

func listen(addr string, id *tlsid.Identity) (net.Listener, error) {
	if strings.HasPrefix(addr, "unix://") {
		return transports.NewSocket(addr)
	}
	return transports.NewTCP(addr, id)
}
