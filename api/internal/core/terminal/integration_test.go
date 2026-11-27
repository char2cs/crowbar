//go:build integration

package terminal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

type pipeConn struct {
	mu        sync.Mutex
	inbox     chan []byte
	outbox    chan []byte
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

func (p *pipeConn) SetWriteDeadline(time.Time) error { return nil }

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

func (p *pipeConn) sendToServer(
	data []byte,
) {
	p.outbox <- data
}

type closedErr struct{}

func (e *closedErr) Error() string { return "connection closed" }

func waitForOutput(
	t *testing.T,
	conn *pipeConn,
	pred func(data string) bool,
) {
	t.Helper()
	for raw := range conn.inbox {
		data, _, ok := terminal.ParseOutputFrame(raw)
		if !ok {
			continue
		}
		if pred(string(data)) {
			return
		}
	}
	t.Fatal("conn closed before the awaited output arrived")
}

func TestIntegration_TwoClientsReceiveOutput(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
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

	require.NoError(t, eng.Write(ctx, sid, []byte("echo hello\n")))

	contains := func(data string) bool {
		for i := 0; i+4 < len(data)+1; i++ {
			if len(data) >= 5 && containsSubstr(data, "hello") {
				return true
			}
		}
		return containsSubstr(data, "hello")
	}

	waitForOutput(t, conn1, contains)
	waitForOutput(t, conn2, contains)

	conn1.Close()
	conn2.Close()
	require.NoError(t, eng.Kill(ctx, sid))

	<-attachDone1
	<-attachDone2
}

func TestIntegration_ReattachSerializedRedraw(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-2", dir, nil)
	require.NoError(t, err)

	conn1 := newPipeConn()
	attachDone1 := make(chan struct{})
	go func() {
		defer close(attachDone1)
		_ = eng.Attach(ctx, sid, conn1)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo ring\n")))

	hasRing := func(data string) bool { return containsSubstr(data, "ring") }
	waitForOutput(t, conn1, hasRing)

	conn1.Close()
	<-attachDone1

	conn2 := newPipeConn()
	attachDone2 := make(chan struct{})
	go func() {
		defer close(attachDone2)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	waitForOutput(t, conn2, hasRing)

	conn2.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone2
}

func TestIntegration_KillClosesClients(t *testing.T) {
	eng := terminal.New()
	defer eng.Shutdown()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-3", dir, nil)
	require.NoError(t, err)

	conn := newPipeConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Kill(ctx, sid))

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
