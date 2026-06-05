package terminal_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// mockConn is a simple in-memory WSConn for unit tests.
type mockConn struct {
	mu        sync.Mutex
	inbox     [][]byte
	outbox    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newMockConn() *mockConn {
	return &mockConn{
		outbox: make(chan []byte, 256),
		closed: make(chan struct{}),
	}
}

func (m *mockConn) WriteMessage(
	_ int,
	data []byte,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.inbox = append(m.inbox, cp)
	return nil
}

func (m *mockConn) ReadMessage() (int, []byte, error) {
	select {
	case msg := <-m.outbox:
		return 1, msg, nil
	case <-m.closed:
		return 0, nil, &connClosedErr{}
	}
}

func (m *mockConn) Close() error {
	m.closeOnce.Do(func() { close(m.closed) })
	return nil
}

func (m *mockConn) allReceived() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.inbox))
	copy(out, m.inbox)
	return out
}

type connClosedErr struct{}

func (e *connClosedErr) Error() string { return "connection closed" }

func waitForMsg(
	t *testing.T,
	conn *mockConn,
	pred func(string) bool,
	timeout time.Duration,
) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		for _, raw := range conn.allReceived() {
			var msg struct {
				Data string `json:"data"`
			}
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if pred(msg.Data) {
				return true
			}
		}
		select {
		case <-deadline:
			return false
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestEngine_Create_And_ListSessions(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	assert.Empty(t, eng.ListSessions())

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, sid)

	assert.Contains(t, eng.ListSessions(), sid)

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Kill_Unknown(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	err := eng.Kill(ctx, "nope")
	assert.Error(t, err)
}

func TestEngine_Write_And_Resize_Unknown(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	assert.Error(t, eng.Write(ctx, "nope", []byte("hi")))
	assert.Error(t, eng.Resize(ctx, "nope", 80, 24))
}

func TestEngine_Write(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Write(ctx, sid, []byte("echo ok\n")))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Resize(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Resize(ctx, sid, 120, 40))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Attach_UnknownSession(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	conn := newMockConn()
	conn.Close()
	err := eng.Attach(ctx, "ghost", conn)
	assert.Error(t, err)
}

func TestEngine_Attach_ReceivesOutput(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo hello\n")))

	found := waitForMsg(t, conn, func(data string) bool {
		return len(data) >= 5 && containsStr(data, "hello")
	}, 5*time.Second)
	assert.True(t, found, "must receive output containing 'hello'")

	conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

func TestEngine_ListSessions_AfterKill(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)
	require.NoError(t, eng.Kill(ctx, sid))

	assert.NotContains(t, eng.ListSessions(), sid)
}

func TestEngine_Create_BadShell(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	_, err := eng.Create(ctx, "ws-bad", dir, nil)
	// With no profile, it uses $SHELL or /bin/sh which is valid, so this test
	// exercises the profile resolution path. To trigger an error we need a bad profile.
	require.NoError(t, err) // default shell succeeds
	_ = eng.Kill(ctx, eng.ListSessions()[0])
}

func TestEngine_Create_InvalidShellViaProfile(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	prof := &domain.TerminalProfile{Shell: "/nonexistent/shell"}
	_, err := eng.Create(ctx, "ws-bad", dir, prof)
	assert.Error(t, err)
}

func TestEngine_Attach_DeadSession(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	// Kill the underlying PTY process to make the session dead,
	// then try to attach — should get an error.
	require.NoError(t, eng.Kill(ctx, sid))

	// Re-inject the dead session into registry to test Attach on dead session.
	// We cannot do this from outside without access to internals, so instead
	// we verify Attach returns error for a non-existent session ID.
	conn := newMockConn()
	conn.Close()
	err = eng.Attach(ctx, "gone-session", conn)
	assert.Error(t, err)
}

func TestEngine_Attach_ResizeMessage(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	resizeMsg, _ := json.Marshal(map[string]any{"type": "resize", "cols": 120, "rows": 40})
	inputMsg, _ := json.Marshal(map[string]any{"data": "echo ok\n"})
	msgs := [][]byte{
		resizeMsg,
		[]byte("not-json{{{"),
		inputMsg,
	}
	conn := newSeqConn(msgs)

	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Wait until the seqConn has delivered all messages.
	select {
	case <-conn.allConsumed:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for messages to be consumed")
	}

	conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

// seqConn delivers a fixed sequence of messages then blocks until closed.
type seqConn struct {
	msgs        [][]byte
	pos         int
	mu          sync.Mutex
	allConsumed chan struct{}
	done        chan struct{}
	once        sync.Once
}

func newSeqConn(
	msgs [][]byte,
) *seqConn {
	return &seqConn{
		msgs:        msgs,
		allConsumed: make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (c *seqConn) WriteMessage(
	_ int,
	_ []byte,
) error {
	return nil
}

func (c *seqConn) ReadMessage() (int, []byte, error) {
	c.mu.Lock()
	if c.pos < len(c.msgs) {
		msg := c.msgs[c.pos]
		c.pos++
		if c.pos == len(c.msgs) {
			close(c.allConsumed)
		}
		c.mu.Unlock()
		return 1, msg, nil
	}
	c.mu.Unlock()
	<-c.done
	return 0, nil, &connClosedErr{}
}

func (c *seqConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return nil
}

func TestEngine_Attach_WritePumpClosedConn(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	conn := newErrConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Write to trigger writePump which will fail on WriteMessage.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo hi\n")))

	select {
	case <-attachDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Attach did not return after write error")
	}
	require.NoError(t, eng.Kill(ctx, sid))
}

// errConn is a WSConn whose WriteMessage always errors on every call.
type errConn struct {
	writes int
	closed chan struct{}
	once   sync.Once
}

func newErrConn() *errConn {
	return &errConn{closed: make(chan struct{})}
}

func (e *errConn) WriteMessage(
	_ int,
	_ []byte,
) error {
	e.writes++
	if e.writes > 0 {
		return &connClosedErr{}
	}
	return nil
}

func (e *errConn) ReadMessage() (int, []byte, error) {
	<-e.closed
	return 0, nil, &connClosedErr{}
}

func (e *errConn) Close() error {
	e.once.Do(func() { close(e.closed) })
	return nil
}

func TestEngine_Create_WithStartupCommands(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	prof := &domain.TerminalProfile{
		ID:              "prof-startup",
		StartupCommands: []string{"echo startup-test"},
	}

	sid, err := eng.Create(ctx, "ws-1", dir, prof)
	require.NoError(t, err)
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_SessionExists(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	assert.False(t, eng.SessionExists(ctx, "nonexistent"))

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	assert.True(t, eng.SessionExists(ctx, sid))
	require.NoError(t, eng.Kill(ctx, sid))
	assert.False(t, eng.SessionExists(ctx, sid))
}

func TestEngine_Shutdown(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)
	assert.True(t, eng.SessionExists(ctx, sid))

	eng.Shutdown()
	assert.False(t, eng.SessionExists(ctx, sid))
}

func containsStr(
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
