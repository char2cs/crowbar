package terminal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeMetaStore struct {
	mu      sync.Mutex
	saved   []terminal.SessionMeta
	deleted []string
	rows    map[string]terminal.SessionMeta
	dir     string
}

func newFakeMetaStore(t *testing.T) *fakeMetaStore {
	t.Helper()
	return &fakeMetaStore{dir: t.TempDir(), rows: make(map[string]terminal.SessionMeta)}
}

func (f *fakeMetaStore) Save(_ context.Context, meta terminal.SessionMeta) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, meta)
	f.rows[meta.SessionID] = meta
	return nil
}

func (f *fakeMetaStore) Delete(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, sessionID)
	delete(f.rows, sessionID)
	return nil
}

func (f *fakeMetaStore) liveRows() []terminal.SessionMeta {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]terminal.SessionMeta, 0, len(f.rows))
	for _, m := range f.rows {
		out = append(out, m)
	}
	return out
}

func (f *fakeMetaStore) hasLiveRow(sid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.rows[sid]
	return ok
}

func (f *fakeMetaStore) StorageDir(_ context.Context, _ string) (string, error) {
	return f.dir, nil
}

func (f *fakeMetaStore) List(_ context.Context) ([]domain.TerminalSession, error) {
	return nil, nil
}

func (f *fakeMetaStore) hasSavedWithState(sid, state string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.saved {
		if m.SessionID == sid && m.State == state {
			return true
		}
	}
	return false
}

func (f *fakeMetaStore) hasDeleted(sid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.deleted {
		if id == sid {
			return true
		}
	}
	return false
}

func bufExists(dir, sid string) bool {
	_, err := os.Stat(filepath.Join(dir, sid+".buf"))
	return err == nil
}

type mockConn struct {
	mu        sync.Mutex
	inbox     [][]byte
	outbox    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	notify    chan struct{}
}

func newMockConn() *mockConn {
	return &mockConn{
		outbox: make(chan []byte, 256),
		closed: make(chan struct{}),
		notify: make(chan struct{}, 1),
	}
}

func (m *mockConn) WriteMessage(
	_ int,
	data []byte,
) error {
	m.mu.Lock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.inbox = append(m.inbox, cp)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
	return nil
}

func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

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
) {
	t.Helper()
	for {
		if connMatches(conn, pred) {
			return
		}
		select {
		case <-conn.notify:
		case <-conn.closed:
			if connMatches(conn, pred) {
				return
			}
			t.Fatal("conn closed before the awaited frame arrived")
		}
	}
}

func connMatches(conn *mockConn, pred func(string) bool) bool {
	for _, raw := range conn.allReceived() {
		data, _, ok := terminal.ParseOutputFrame(raw)
		if !ok {
			continue
		}
		if pred(string(data)) {
			return true
		}
	}
	return false
}

func TestEngine_Kill_Unknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	err := eng.Kill(ctx, "nope")
	assert.Error(t, err)
}

func TestEngine_Attach_UnknownSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	conn := newMockConn()
	_ = conn.Close()
	err := eng.Attach(ctx, "ghost", conn)
	assert.Error(t, err)
}

func TestEngine_Attach_ReceivesOutput(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo hello\n")))

	waitForMsg(t, conn, func(data string) bool {
		return len(data) >= 5 && containsStr(data, "hello")
	})

	_ = conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

func TestEngine_Attach_DeadSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Kill(ctx, sid))

	conn := newMockConn()
	_ = conn.Close()
	err = eng.Attach(ctx, "gone-session", conn)
	assert.Error(t, err)
}

func TestEngine_Attach_ResizeMessage(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
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

	<-conn.allConsumed

	_ = conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

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

func (c *seqConn) SetWriteDeadline(time.Time) error { return nil }

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
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	conn := newErrConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo hi\n")))

	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

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

func (e *errConn) SetWriteDeadline(time.Time) error { return nil }

func (e *errConn) ReadMessage() (int, []byte, error) {
	<-e.closed
	return 0, nil, &connClosedErr{}
}

func (e *errConn) Close() error {
	e.once.Do(func() { close(e.closed) })
	return nil
}

func TestEngine_Shutdown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
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

func TestEngine_LoadPlaceholder_ThenAttach_Restores(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := "ph-sess-attach"
	marker := "pre-restart-marker"
	scrollback := []byte("CRWB1 80 24 0 10000\n$ echo " + marker + "\r\n" + marker + "\r\n")

	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-ph-attach",
		CWD:       dir,
		Shell:     "/bin/sh",
		ProfileID: "",
	}, scrollback))

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, marker)
	})

	_ = conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Detach_PersistsScrollbackAndMeta(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "chat-detach", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo detach-marker\n")))
	waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, "detach-marker")
	})

	_ = conn.Close()
	<-attachDone

	assert.True(t, bufExists(store.dir, sid), ".buf file must exist after last-client detach")
	assert.True(t, store.hasSavedWithState(sid, "detached"),
		"meta must be saved with state=detached after last-client detach")

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Suspend_WithConnectedClient_NoOp(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-susp-client", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	require.NoError(t, eng.Suspend(ctx, sid))
	assert.True(t, eng.SessionExists(ctx, sid), "session must still exist after no-op suspend")

	assert.NoError(t, eng.Write(ctx, sid, []byte("echo still-alive\n")))

	_ = conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Suspend_MakesPlaceholder(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-susp", dir)
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	assert.True(t, eng.SessionExists(ctx, sid), "suspended session must still be in registry")
	assert.True(t, bufExists(store.dir, sid), ".buf must exist after suspend")

	eng.Shutdown()
	assert.True(t, bufExists(store.dir, sid), ".buf must survive the suspended process's reap")
	assert.True(t, store.hasLiveRow(sid), "the suspended row must survive the reap")
}

func TestEngine_Suspend_ThenAttach_Restores(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-restore", dir)

	conn1 := newMockConn()
	attach1Done := make(chan struct{})
	go func() {
		defer close(attach1Done)
		_ = eng.Attach(ctx, sid, conn1)
	}()
	require.NoError(t, eng.Write(ctx, sid, []byte("echo pre-suspend\n")))
	waitForMsg(t, conn1, func(d string) bool { return containsStr(d, "pre-suspend") })
	_ = conn1.Close()

	<-attach1Done
	st, ok := eng.StateOf(sid)
	require.True(t, ok)
	require.Equal(t, "detached", st, "the session must be detached once its last client has gone")

	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	conn2 := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "pre-suspend")
	})

	require.NoError(t, eng.Write(ctx, sid, []byte("echo post-restore\n")))
	waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "post-restore")
	})

	_ = conn2.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Kill_CleanReap(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "chat-kill", dir, nil)
	require.NoError(t, err)

	bufFile := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufFile, []byte("scrollback"), 0o644))

	require.NoError(t, eng.Kill(ctx, sid))
	eng.Shutdown()

	assert.False(t, eng.SessionExists(ctx, sid), "session must be absent after kill")
	assert.True(t, store.hasDeleted(sid), "meta must be deleted by reapOnDone")
	assert.False(t, bufExists(store.dir, sid), ".buf must be deleted by reapOnDone")
}
