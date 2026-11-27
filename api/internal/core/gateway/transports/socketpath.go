package transports

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

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
