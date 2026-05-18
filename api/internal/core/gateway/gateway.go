package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/rabbytesoftware/crowbar/api/internal/core/gateway/transports"
)

func New(host string) (net.Listener, error) {
	switch {
	case strings.HasPrefix(host, "unix://"):
		return transports.NewSocket(host)
	case strings.HasPrefix(host, "tcp://"):
		return transports.NewTCP(host)
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}
