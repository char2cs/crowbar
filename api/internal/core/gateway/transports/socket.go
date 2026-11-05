package transports

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

// chmod is indirected so tests can simulate a chmod failure.
var chmod = os.Chmod

// ErrDaemonRunning is returned when a crowbar daemon is already listening on
// the socket path — the stale-socket check dialed it and got a live answer, so
// binding would steal the address from a healthy daemon.
var ErrDaemonRunning = errors.New("gateway: socket: daemon already running")

// NewSocket binds the daemon's Unix socket for host ("unix://[path]"; empty path is
// the home's SocketPath), creating its directory. A stale socket is removed, a live
// one yields ErrDaemonRunning, and Close removes the socket file.
func NewSocket(
	host string,
) (net.Listener, error) {
	path, err := SocketPath(host)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("gateway: socket: create dir for %s: %w", path, err)
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
			return nil, errors.Join(fmt.Errorf("gateway: socket: chmod %s: %w", path, err), ln.Close())
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

	if closeErr := conn.Close(); closeErr != nil {
		return errors.Join(ErrDaemonRunning, closeErr)
	}
	return ErrDaemonRunning
}
