package transports

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
)

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
