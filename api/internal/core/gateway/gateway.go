package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

func New(host string, id *tlsid.Identity) (net.Listener, error) {
	switch {
	case strings.HasPrefix(host, "unix://"):
		return transports.NewSocket(host)
	case strings.HasPrefix(host, "tcp://"):
		return transports.NewTCP(host, id)
	default:
		return nil, fmt.Errorf("unsupported host scheme %q: use unix:// or tcp://", host)
	}
}
