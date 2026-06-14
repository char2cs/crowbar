package transports

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultSocketName = "crowbar.sock"

// chmod is indirected so tests can simulate a chmod failure.
var chmod = os.Chmod

// ErrDaemonRunning is returned when a crowbar daemon is already listening on
// the socket path — the stale-socket check dialed it and got a live answer, so
// binding would steal the address from a healthy daemon.
var ErrDaemonRunning = errors.New("gateway: socket: daemon already running")

// NewSocket binds a Unix domain socket for the daemon. host is the full
// "unix://[path]" string; an empty path resolves to ~/.crowbar/crowbar.sock so
// every client (CLI, the desktop proxy) can find the daemon at a fixed,
// well-known location instead of guessing a per-process path.
//
// A stale socket left behind by a crashed daemon is removed before binding; a
// socket that still has a live daemon behind it yields ErrDaemonRunning rather
// than being clobbered. The returned listener removes the socket file on Close
// so a clean shutdown never leaves a stale socket for the next launch.
func NewSocket(
	host string,
) (net.Listener, error) {
	path, err := socketPath(strings.TrimPrefix(host, "unix://"))
	if err != nil {
		return nil, err
	}

	if err := handleStale(path); err != nil {
		return nil, err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("gateway: socket: listen %s: %w", path, err)
	}

	if runtime.GOOS != "windows" {
		if err := chmod(path, 0o600); err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("gateway: socket: chmod %s: %w", path, err)
		}
	}

	return &socketListener{Listener: ln, path: path}, nil
}

// handleStale clears a socket file left over from a crashed daemon, but refuses
// to remove one that still has a live daemon behind it (dialing succeeds).
func handleStale(
	path string,
) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		return os.Remove(path)
	}

	_ = conn.Close()
	return ErrDaemonRunning
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
