package transports

import (
	"net"
	"strings"
)

func NewTCP(host string) (net.Listener, error) {
	addr := strings.TrimPrefix(host, "tcp://")
	return net.Listen("tcp", addr)
}
