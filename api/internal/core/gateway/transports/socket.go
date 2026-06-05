package transports

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

const defaultSocketName = "crowbar.sock"

func NewSocket(
	host string,
) (net.Listener, error) {
	path, err := socketPath(strings.TrimPrefix(host, "unix://"))
	if err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

func socketPath(
	path string,
) (string, error) {
	if path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".crowbar")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, defaultSocketName), nil
}
