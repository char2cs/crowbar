package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

func New(host, socket string) ([]net.Listener, error) {
	switch {
	case host == "unix://":
		return bind("unix://" + socket)
	case strings.HasPrefix(host, "unix://"), strings.HasPrefix(host, "tcp://") && !Exposed(host):
		return bind(host)
	case Exposed(host):
		return nil, fmt.Errorf("gateway: %q is network-exposed and is served by the device listener", host)
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}

func Exposed(host string) bool {
	return strings.HasPrefix(host, "tcp://") && transports.IsNetworkExposed(host)
}

func bind(addr string) ([]net.Listener, error) {
	var (
		ln  net.Listener
		err error
	)
	if strings.HasPrefix(addr, "unix://") {
		ln, err = transports.NewSocket(addr)
	} else {
		ln, err = transports.NewTCP(addr, nil)
	}
	if err != nil {
		return nil, err
	}
	return []net.Listener{ln}, nil
}
