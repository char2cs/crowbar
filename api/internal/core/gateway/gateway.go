package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
)

// New listens on host: "tcp://addr", "unix:///path", or "unix://", which binds
// socket, the socket of the home the caller holds, so the two never disagree.
func New(
	host string,
	socket string,
) (net.Listener, error) {
	switch {
	case host == "unix://":
		return transports.NewSocket(host + socket)
	case strings.HasPrefix(host, "unix://"):
		return transports.NewSocket(host)
	case strings.HasPrefix(host, "tcp://"):
		return transports.NewTCP(host)
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}
