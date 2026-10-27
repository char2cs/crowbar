package transports

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
)

// socketListener wraps a Unix domain net.Listener and removes the socket file
// on Close, so a clean shutdown never leaves a stale socket behind for the next
// daemon launch to trip over.
type socketListener struct {
	net.Listener
	path string
}

func (l *socketListener) Close() error {
	err := l.Listener.Close()
	if rmErr := os.Remove(l.path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		err = errors.Join(err, fmt.Errorf("gateway: socket: remove %s: %w", l.path, rmErr))
	}
	return err
}
