package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

// New binds every listener the daemon serves for host (see bindAddrs),
// closing any it already bound if a later one fails.
func New(host string, id *tlsid.Identity) ([]net.Listener, error) {
	addrs, err := bindAddrs(host)
	if err != nil {
		return nil, err
	}
	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		ln, err := listen(addr, id)
		if err != nil {
			for _, bound := range listeners {
				_ = bound.Close()
			}
			return nil, err
		}
		listeners = append(listeners, ln)
	}
	return listeners, nil
}

// bindAddrs lists the addresses New binds for host, in bind order. A
// network-exposed tcp:// host also binds the local unix socket: `crowbar pair`
// only dials that socket, and pairing must work from the daemon's own host.
func bindAddrs(host string) ([]string, error) {
	switch {
	case strings.HasPrefix(host, "unix://"):
		return []string{host}, nil
	case strings.HasPrefix(host, "tcp://") && transports.IsNetworkExposed(host):
		return []string{"unix://", host}, nil
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
