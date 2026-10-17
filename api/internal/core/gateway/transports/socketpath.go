package transports

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// SocketPath resolves the daemon's unix socket path from a host string
// ("unix://" or "unix:///abs/path"). With no explicit path it is the socket of
// the resolved Crowbar home. Shared by the daemon (NewSocket) and unix-socket
// clients (e.g. `crowbar hook`).
func SocketPath(host string) (string, error) {
	if path := strings.TrimPrefix(host, "unix://"); path != "" {
		return path, nil
	}
	h, err := home.Resolve()
	if err != nil {
		return "", fmt.Errorf("gateway: socket: resolve home: %w", err)
	}
	return h.Socket(), nil
}
