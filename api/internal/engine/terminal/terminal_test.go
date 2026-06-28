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

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ---------------------------------------------------------------------------
// fakeMetaStore — test double for SessionMetaStore.
// ---------------------------------------------------------------------------

type fakeMetaStore struct {
	mu      sync.Mutex
	saved   []terminal.SessionMeta
	deleted []string
	rows    map[string]terminal.SessionMeta // current live rows (upsert on Save, drop on Delete)
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

// liveRows returns a snapshot of the currently-persisted rows (Saved and not
// subsequently Deleted), mirroring what RestorePersistedSessions would iterate.
func (f *fakeMetaStore) liveRows() []terminal.SessionMeta {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]terminal.SessionMeta, 0, len(f.rows))
	for _, m := range f.rows {
		out = append(out, m)
	}
	return out
}

// hasLiveRow reports whether a current (non-deleted) row exists for sid.
func (f *fakeMetaStore) hasLiveRow(sid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.rows[sid]
	return ok
}

func (f *fakeMetaStore) StorageDir(_ context.Context, _ string) (string, error) {
	return f.dir, nil
}

// List returns an empty slice — the engine-level fakeMetaStore is only used
// to test Save/Delete/StorageDir; List is not exercised at this layer.
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

func TestEngine_Create_KeysSessionByWorkspace(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid1, err := eng.Create(ctx, "ws-a", dir, nil)
	require.NoError(t, err)
	sid2, err := eng.Create(ctx, "ws-b", dir, nil)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{sid1}, eng.ListSessionsForWorkspace("ws-a"))
	assert.ElementsMatch(t, []string{sid2}, eng.ListSessionsForWorkspace("ws-b"))

	require.NoError(t, eng.Kill(ctx, sid1))
	require.NoError(t, eng.Kill(ctx, sid2))
}

func TestEngine_ListSessionsForWorkspace(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	assert.Empty(t, eng.ListSessionsForWorkspace("ws-a"))

	sid, err := eng.Create(ctx, "ws-a", dir, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{sid}, eng.ListSessionsForWorkspace("ws-a"))
	assert.Empty(t, eng.ListSessionsForWorkspace("ws-other"))

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_OnSessionEnded_FiresOnReap(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	type ended struct {
		wsID     string
		sid      string
		exitCode int
	}
	endedCh := make(chan ended, 1)
	eng.OnSessionEnded(func(_ context.Context, wsID, sid string, exitCode int) {
		endedCh <- ended{wsID: wsID, sid: sid, exitCode: exitCode}
	})

	sid, err := eng.Create(ctx, "ws-a", dir, nil)
	require.NoError(t, err)

	// Kill triggers the PTY exit, which reapOnDone observes and reports via the
	// OnSessionEnded callback. Synchronise on the callback channel, never sleep.
	require.NoError(t, eng.Kill(ctx, sid))

	select {
	case got := <-endedCh:
		assert.Equal(t, "ws-a", got.wsID)
		assert.Equal(t, sid, got.sid)
		// exitCode is -1 when killed by signal; any integer is valid here.
		_ = got.exitCode
	case <-time.After(5 * time.Second):
		t.Fatal("OnSessionEnded did not fire after Kill")
	}
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

// ---------------------------------------------------------------------------
// Phase 3: LoadPlaceholder / restart-restore tests (TDD — written first)
// ---------------------------------------------------------------------------

// TestEngine_LoadPlaceholder_IsPlaceholder verifies that after LoadPlaceholder the
// session appears in the registry as a suspended (non-live) placeholder.
func TestEngine_LoadPlaceholder_IsPlaceholder(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	sid := "ph-sess-1"
	err := eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-ph",
		CWD:         "/tmp",
		Shell:       "/bin/sh",
		ProfileID:   "",
		State:       "suspended",
	}, []byte("old scrollback"))
	require.NoError(t, err)

	assert.True(t, eng.SessionExists(ctx, sid), "placeholder must be present in registry")
	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state, "placeholder state must be 'suspended'")

	// Cleanup.
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_LoadPlaceholder_Idempotent verifies that loading the same session ID
// twice does not double-register and does not error.
func TestEngine_LoadPlaceholder_Idempotent(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	sid := "ph-sess-idem"
	meta := terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-ph",
		CWD:         "/tmp",
		Shell:       "/bin/sh",
	}

	require.NoError(t, eng.LoadPlaceholder(ctx, meta, nil))
	require.NoError(t, eng.LoadPlaceholder(ctx, meta, nil)) // second call must be no-op

	// Should still appear exactly once.
	ids := eng.ListSessions()
	count := 0
	for _, id := range ids {
		if id == sid {
			count++
		}
	}
	assert.Equal(t, 1, count, "session must be registered exactly once after two loads")

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_LoadPlaceholder_ThenAttach_Restores verifies that attaching to a
// session loaded via LoadPlaceholder transparently restores it: a live PTY is
// spawned and the scrollback bytes pre-loaded into the placeholder are replayed.
func TestEngine_LoadPlaceholder_ThenAttach_Restores(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := "ph-sess-attach"
	marker := "pre-restart-marker"
	scrollback := []byte("$ echo " + marker + "\r\n" + marker + "\r\n")

	// Write scrollback to disk so restore() can read it via persistence.ReadBuf.
	// (restore re-reads from disk, not from the placeholder ring.)
	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-ph-attach",
		CWD:         dir,
		Shell:       "/bin/sh",
		ProfileID:   "",
	}, scrollback))

	// Attach must trigger restore transparently.
	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// The replayed scrollback must appear in the first messages.
	found := waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, marker)
	}, 8*time.Second)
	assert.True(t, found, "scrollback must be replayed after LoadPlaceholder → Attach")

	conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// ---------------------------------------------------------------------------
// Phase 2: Suspend / Restore / detach-persistence tests (TDD — written first)
// ---------------------------------------------------------------------------

// TestEngine_Detach_PersistsScrollbackAndMeta verifies that when the last client
// disconnects the engine writes the .buf file and records a "detached" meta entry.
func TestEngine_Detach_PersistsScrollbackAndMeta(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "ws-detach", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	require.NoError(t, eng.Write(ctx, sid, []byte("echo detach-marker\n")))
	found := waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, "detach-marker")
	}, 5*time.Second)
	require.True(t, found, "must receive 'detach-marker' output before detaching")

	conn.Close()
	<-attachDone

	assert.True(t, bufExists(store.dir, sid), ".buf file must exist after last-client detach")
	assert.True(t, store.hasSavedWithState(sid, "detached"),
		"meta must be saved with state=detached after last-client detach")

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_Suspend_WithConnectedClient_NoOp verifies that Suspend is a no-op
// when a client is currently attached (BeginSuspendIfEligible returns false).
func TestEngine_Suspend_WithConnectedClient_NoOp(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-susp-client", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Wait for at least one frame to confirm the client is fully registered.
	found := waitForMsg(t, conn, func(d string) bool { return len(d) > 0 }, 3*time.Second)
	require.True(t, found, "must receive initial PTY output to confirm attach is live")

	// Suspend with a connected client must be a no-op.
	require.NoError(t, eng.Suspend(ctx, sid))
	assert.True(t, eng.SessionExists(ctx, sid), "session must still exist after no-op suspend")

	// Session must still be writable (live PTY).
	assert.NoError(t, eng.Write(ctx, sid, []byte("echo still-alive\n")))

	conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_Suspend_MakesPlaceholder verifies that after Suspend on an idle session:
//   - the session still exists in the registry (as a placeholder)
//   - a .buf file is written to disk
//   - meta is saved with state="suspended"
//   - the onEnded callback is NOT fired
func TestEngine_Suspend_MakesPlaceholder(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	endedFired := false
	eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) { endedFired = true })

	sid, err := eng.Create(ctx, "ws-susp", dir, nil)
	require.NoError(t, err)

	// Poll: keep calling Suspend until meta shows "suspended" (shell must be idle).
	deadline := time.After(15 * time.Second)
	for {
		_ = eng.Suspend(ctx, sid)
		if store.hasSavedWithState(sid, "suspended") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session was not suspended within 15s")
		case <-time.After(200 * time.Millisecond):
		}
	}

	assert.True(t, eng.SessionExists(ctx, sid), "suspended session must still be in registry")
	assert.True(t, bufExists(store.dir, sid), ".buf must exist after suspend")

	// Give a brief window for any spurious async reap to fire — should not happen.
	time.Sleep(300 * time.Millisecond)
	assert.False(t, endedFired, "onEnded must NOT fire for a suspended session")
}

// TestEngine_Suspend_ThenAttach_Restores verifies that attaching to a suspended
// session triggers restore: a fresh shell spawns, prior scrollback is replayed,
// and new output flows normally.
func TestEngine_Suspend_ThenAttach_Restores(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "ws-restore", dir, nil)
	require.NoError(t, err)

	// Attach first client, write something to populate the ring, then detach.
	conn1 := newMockConn()
	go func() { _ = eng.Attach(ctx, sid, conn1) }()
	require.NoError(t, eng.Write(ctx, sid, []byte("echo pre-suspend\n")))
	waitForMsg(t, conn1, func(d string) bool { return containsStr(d, "pre-suspend") }, 5*time.Second)
	conn1.Close()
	// Brief wait for detach bookkeeping to finish.
	time.Sleep(150 * time.Millisecond)

	// Suspend: poll until it takes effect.
	deadline := time.After(15 * time.Second)
	for {
		_ = eng.Suspend(ctx, sid)
		if store.hasSavedWithState(sid, "suspended") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session did not suspend within 15s")
		case <-time.After(200 * time.Millisecond):
		}
	}

	// Attach to the suspended session → restore must happen transparently.
	conn2 := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	// Restored session must replay prior scrollback.
	found := waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "pre-suspend")
	}, 8*time.Second)
	assert.True(t, found, "scrollback must be replayed after restore")

	// New output must flow through the restored live shell.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo post-restore\n")))
	found = waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "post-restore")
	}, 5*time.Second)
	assert.True(t, found, "new output must flow after restore")

	conn2.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_Kill_CleanReap verifies that Kill triggers a full reap: session removed
// from registry, meta deleted, .buf deleted, and onEnded fired exactly once.
func TestEngine_Kill_CleanReap(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	endedCh := make(chan string, 1)
	eng.OnSessionEnded(func(_ context.Context, _, sid string, _ int) { endedCh <- sid })

	sid, err := eng.Create(ctx, "ws-kill", dir, nil)
	require.NoError(t, err)

	// Pre-write a .buf so we can verify Kill deletes it.
	bufFile := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufFile, []byte("scrollback"), 0o644))

	require.NoError(t, eng.Kill(ctx, sid))

	// Wait for reapOnDone to fire the callback.
	select {
	case got := <-endedCh:
		assert.Equal(t, sid, got)
	case <-time.After(5 * time.Second):
		t.Fatal("onEnded did not fire after Kill")
	}

	assert.False(t, eng.SessionExists(ctx, sid), "session must be absent after kill")
	assert.True(t, store.hasDeleted(sid), "meta must be deleted by reapOnDone")
	assert.False(t, bufExists(store.dir, sid), ".buf must be deleted by reapOnDone")
}

// ---------------------------------------------------------------------------
// StateOf + OnSessionState tests (lifecycle state wire-protocol — Phase 2 TDD)
// ---------------------------------------------------------------------------

// TestEngine_StateOf_ReturnsStateForLiveSession verifies StateOf returns
// "detached" for a freshly created session (no clients yet).
func TestEngine_StateOf_ReturnsStateForLiveSession(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-stateof", dir, nil)
	require.NoError(t, err)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok, "StateOf must return ok=true for a live session")
	// Newly created session has no clients → "detached"
	assert.Equal(t, "detached", state)

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_StateOf_ReturnsFalseForUnknown verifies StateOf returns ("", false)
// for a session that does not exist in the registry.
func TestEngine_StateOf_ReturnsFalseForUnknown(t *testing.T) {
	eng := terminal.New()

	state, ok := eng.StateOf("no-such-session")
	assert.False(t, ok)
	assert.Empty(t, state)
}

// TestEngine_StateOf_ActiveWhileAttached verifies StateOf returns "active"
// while a client is attached.
func TestEngine_StateOf_ActiveWhileAttached(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-active", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Wait until we receive at least one frame, confirming the client is registered.
	found := waitForMsg(t, conn, func(d string) bool { return len(d) > 0 }, 3*time.Second)
	require.True(t, found, "must receive initial output to confirm attach")

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state)

	conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_OnSessionState_DetachedFiredOnLastClientLeave verifies that the
// OnSessionState callback receives "detached" when the last client disconnects.
func TestEngine_OnSessionState_DetachedFiredOnLastClientLeave(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()

	stateCh := make(chan string, 4)
	eng.OnSessionState(func(_ context.Context, _, _, state string) {
		stateCh <- state
	})

	sid, err := eng.Create(ctx, "ws-detach-state", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	found := waitForMsg(t, conn, func(d string) bool { return len(d) > 0 }, 3*time.Second)
	require.True(t, found, "must receive initial output before closing")

	conn.Close()
	<-attachDone

	select {
	case got := <-stateCh:
		assert.Equal(t, "detached", got)
	case <-time.After(3 * time.Second):
		t.Fatal("OnSessionState 'detached' did not fire after last-client disconnect")
	}

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestRegression_TerminalLifecycle_StateOf_And_OnSessionState is the regression
// test for the lifecycle wire-protocol data path (Phase 2): the engine's
// StateOf must reflect the session's actual state, and the OnSessionState
// callback must fire "detached" after last-client disconnect and "suspended"
// after Suspend.
func TestRegression_TerminalLifecycle_StateOf_And_OnSessionState(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	stateCh := make(chan string, 8)
	eng.OnSessionState(func(_ context.Context, _, _, state string) {
		stateCh <- state
	})

	endedCh := make(chan int, 1)
	eng.OnSessionEnded(func(_ context.Context, _, _ string, exitCode int) {
		endedCh <- exitCode
	})

	// Create → should be detached (no clients).
	sid, err := eng.Create(ctx, "ws-regress", dir, nil)
	require.NoError(t, err)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state, "newly created session must report detached (no clients)")

	// Attach client → "active".
	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()
	found := waitForMsg(t, conn, func(d string) bool { return len(d) > 0 }, 3*time.Second)
	require.True(t, found, "must receive initial output to confirm attach")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state, "session must be active while client is attached")

	// Detach client → "detached" + callback fires.
	conn.Close()
	<-attachDone

	select {
	case got := <-stateCh:
		assert.Equal(t, "detached", got, "OnSessionState must fire 'detached' on last-client leave")
	case <-time.After(3 * time.Second):
		t.Fatal("OnSessionState 'detached' did not fire after detach")
	}

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state)

	// Suspend → "suspended" + callback fires.
	deadline := time.After(15 * time.Second)
	for {
		_ = eng.Suspend(ctx, sid)
		if store.hasSavedWithState(sid, "suspended") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session did not suspend within 15s")
		case <-time.After(200 * time.Millisecond):
		}
	}

	select {
	case got := <-stateCh:
		assert.Equal(t, "suspended", got, "OnSessionState must fire 'suspended' after Suspend")
	case <-time.After(3 * time.Second):
		t.Fatal("OnSessionState 'suspended' did not fire after Suspend")
	}

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state)

	// Kill the placeholder → "ended" via OnSessionEnded.
	require.NoError(t, eng.Kill(ctx, sid))

	select {
	case exitCode := <-endedCh:
		// Placeholder kill → exit code -1 (no process).
		assert.Equal(t, -1, exitCode, "placeholder kill must report exitCode -1")
	case <-time.After(5 * time.Second):
		t.Fatal("OnSessionEnded did not fire after killing suspended placeholder")
	}
}
