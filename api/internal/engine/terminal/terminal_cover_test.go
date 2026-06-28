package terminal

// Internal tests that need direct access to unexported types to cover
// defensive code paths unreachable from the external test package.

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// deadConn is a minimal WSConn that closes immediately on ReadMessage.
type deadConn struct{}

func (d *deadConn) WriteMessage(_ int, _ []byte) error { return nil }
func (d *deadConn) ReadMessage() (int, []byte, error)  { return 0, nil, io.EOF }
func (d *deadConn) Close() error                       { return nil }

// TestAttach_PlaceholderWithBadShell_ReturnsError covers the restore error path:
// the session IS in the registry as a placeholder, but the stored shell binary
// does not exist, so restore (spawn) fails and Attach must return an error.
// (Previously this test injected a killed session and expected s.Attach() to error
// because done was closed. With restore-aware Attach, a not-live session triggers
// restore instead, so we exercise the restore-failure path instead.)
func TestAttach_PlaceholderWithBadShell_ReturnsError(t *testing.T) {
	// Create a placeholder with an invalid shell so spawn fails during restore.
	ph := session.NewPlaceholder("ph-bad", "/nonexistent/shell/binary", t.TempDir(), "", nil)

	eng := New().(*terminalEngine)
	eng.reg.Add("ph-bad", "ws-bad", ph)

	err := eng.Attach(context.Background(), "ph-bad", &deadConn{})
	assert.Error(t, err, "Attach on a placeholder with a bad shell must return an error")
}

// TestFireEnded_NilCallback covers the no-callback short-circuit: fireEnded must
// be a no-op (and must not record the session as ended) when no callback is set.
func TestFireEnded_NilCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	eng.fireEnded(context.Background(), "ws", "s1", 0)
	assert.NotContains(t, eng.endedOnce, "s1")
}

// TestFireEnded_FiresExactlyOnce covers the duplicate guard: a second fireEnded
// for the same session id must not re-invoke the callback.
func TestFireEnded_FiresExactlyOnce(t *testing.T) {
	eng := New().(*terminalEngine)
	var calls int
	eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) { calls++ })

	eng.fireEnded(context.Background(), "ws", "s1", 0)
	eng.fireEnded(context.Background(), "ws", "s1", 0)

	assert.Equal(t, 1, calls)
}

// TestFireState_NilCallback covers the nil-safe guard: fireState must be a
// no-op when no OnSessionState callback is registered.
func TestFireState_NilCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	// Must not panic.
	eng.fireState(context.Background(), "ws", "s1", "detached")
}

// TestFireState_FiresCallback verifies fireState invokes the registered callback
// with the correct arguments.
func TestFireState_FiresCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	type call struct {
		ws    string
		sid   string
		state string
	}
	ch := make(chan call, 2)
	eng.OnSessionState(func(_ context.Context, ws, sid, state string) {
		ch <- call{ws, sid, state}
	})

	eng.fireState(context.Background(), "ws-1", "s-1", "detached")
	eng.fireState(context.Background(), "ws-1", "s-1", "suspended")

	got1 := <-ch
	assert.Equal(t, call{"ws-1", "s-1", "detached"}, got1)
	got2 := <-ch
	assert.Equal(t, call{"ws-1", "s-1", "suspended"}, got2)
}
