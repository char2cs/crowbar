package transports

import (
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
	_ = os.Remove(l.path)
	return err
}
