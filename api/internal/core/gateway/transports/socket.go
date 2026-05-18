package transports

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

const defaultSocketName = "crowbar.sock"

func NewSocket(host string) (net.Listener, error) {
	path := strings.TrimPrefix(host, "unix://")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(home, ".crowbar")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		path = filepath.Join(dir, defaultSocketName)
	}
	_ = os.Remove(path)
	return net.Listen("unix", path)
}
