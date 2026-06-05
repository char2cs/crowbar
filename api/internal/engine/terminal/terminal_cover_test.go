package terminal

// Internal tests that need direct access to unexported types to cover
// defensive code paths unreachable from the external test package.

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// deadConn is a minimal WSConn that closes immediately on ReadMessage.
type deadConn struct{}

func (d *deadConn) WriteMessage(_ int, _ []byte) error { return nil }
func (d *deadConn) ReadMessage() (int, []byte, error)  { return 0, nil, io.EOF }
func (d *deadConn) Close() error                       { return nil }

// TestAttach_SessionDeadInRegistry covers the s.Attach() error path: the
// session IS in the registry but its done channel is already closed.
func TestAttach_SessionDeadInRegistry(t *testing.T) {
	dir := t.TempDir()
	s, err := session.New("dead-id", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)

	// Kill the session and wait until it is fully terminated.
	s.Kill()
	<-s.Done()

	// Inject the dead session directly into a fresh engine's registry.
	eng := New().(*terminalEngine)
	eng.reg.Add("dead-id", s)

	err = eng.Attach(context.Background(), "dead-id", &deadConn{})
	assert.Error(t, err)
}
