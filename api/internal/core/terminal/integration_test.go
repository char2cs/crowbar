//go:build integration

package terminal_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// pipeConn is a pipe-based WSConn for testing without network I/O or timing sleeps.
type pipeConn struct {
	mu        sync.Mutex
	inbox     chan []byte // messages sent by the server to this "client"
	outbox    chan []byte // messages sent by the "client" to the server
	closed    chan struct{}
	closeOnce sync.Once
}

func newPipeConn() *pipeConn {
	return &pipeConn{
		inbox:  make(chan []byte, 256),
		outbox: make(chan []byte, 256),
		closed: make(chan struct{}),
	}
}

func (p *pipeConn) WriteMessage(
	_ int,
	data []byte,
) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case p.inbox <- cp:
	case <-p.closed:
	}
	return nil
}

func (p *pipeConn) ReadMessage() (int, []byte, error) {
	select {
	case msg := <-p.outbox:
		return 1, msg, nil
	case <-p.closed:
		return 0, nil, &closedErr{}
	}
}

func (p *pipeConn) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

// sendToServer enqueues a message as if the client typed it.
func (p *pipeConn) sendToServer(
	data []byte,
) {
	p.outbox <- data
}

type closedErr struct{}

func (e *closedErr) Error() string { return "connection closed" }

// waitForOutput blocks until a frame the engine fanned out to conn satisfies pred.
//
// conn.inbox is fed by the engine's own WriteMessage, so receiving from it is a real signal
// and the wait needs no deadline: between frames there is by definition nothing new to test.
// A predicate that is never satisfied means the engine never produced the output the test was
// waiting for — a hang, which `go test -timeout` reports with the full goroutine dump, rather
// than a bare "conn1 must see 'hello'" with no indication of where it got stuck.
func waitForOutput(
	t *testing.T,
	conn *pipeConn,
	pred func(data string) bool,
) {
	t.Helper()
	for raw := range conn.inbox {
		var msg struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if pred(msg.Data) {
			return
		}
	}
	t.Fatal("conn closed before the awaited output arrived")
}

func TestIntegration_TwoClientsReceiveOutput(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown() // stop the maintenance goroutine so it can't race a later test's limit-var writes
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	conn1 := newPipeConn()
	conn2 := newPipeConn()

	attachDone1 := make(chan struct{})
	attachDone2 := make(chan struct{})

	go func() {
		defer close(attachDone1)
		_ = eng.Attach(ctx, sid, conn1)
	}()

	go func() {
		defer close(attachDone2)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	// Give goroutines time to register (no sleep; use a ready signal via attach).
	// We check output reactively.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo hello\n")))

	contains := func(data string) bool {
		for i := 0; i+4 < len(data)+1; i++ {
			if len(data) >= 5 && containsSubstr(data, "hello") {
				return true
			}
		}
		return containsSubstr(data, "hello")
	}

	waitForOutput(t, conn1, contains) // blocks on the fan-out signal — conn1 must see 'hello'
	waitForOutput(t, conn2, contains) // blocks on the fan-out signal — conn2 must see 'hello'

	conn1.Close()
	conn2.Close()
	require.NoError(t, eng.Kill(ctx, sid))

	<-attachDone1
	<-attachDone2
}

func TestIntegration_ReattachSerializedRedraw(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown() // stop the maintenance goroutine so it can't race a later test's limit-var writes
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-2", dir, nil)
	require.NoError(t, err)

	conn1 := newPipeConn()
	attachDone1 := make(chan struct{})
	go func() {
		defer close(attachDone1)
		_ = eng.Attach(ctx, sid, conn1)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo ring\n")))

	hasRing := func(data string) bool { return containsSubstr(data, "ring") }
	waitForOutput(t, conn1, hasRing) // blocks on the fan-out signal — first client must see 'ring'

	conn1.Close()
	<-attachDone1

	// Second client attaches after the screen model has captured the output.
	// On attach the session serializes its current screen state and redraws it
	// to the new client, so the reattaching client sees the prior output.
	conn2 := newPipeConn()
	attachDone2 := make(chan struct{})
	go func() {
		defer close(attachDone2)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	waitForOutput(t, conn2, hasRing) // blocks on the fan-out signal — serialized redraw must contain 'ring'

	conn2.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone2
}

func TestIntegration_KillClosesClients(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown() // stop the maintenance goroutine so it can't race a later test's limit-var writes
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-3", dir, nil)
	require.NoError(t, err)

	conn := newPipeConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Kill(ctx, sid))

	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-attachDone
}

func containsSubstr(
	s string,
	sub string,
) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
