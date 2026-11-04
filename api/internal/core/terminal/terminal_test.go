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
	// notify is a 1-buffered, coalescing edge published on every frame the engine fans out
	// to this conn. WriteMessage IS the engine telling us it produced output — that is a
	// real signal, and it was previously thrown away in favour of polling inbox on a 10 ms
	// timer. Waiters now block on this and re-scan inbox on each edge, so they wake exactly
	// when there is something new to look at.
	notify chan struct{}
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
	// Non-blocking: the engine's fan-out must never be paced by whether a test is
	// listening. inbox retains every frame, so a coalesced edge loses no data — it only
	// says "look again".
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

// waitForMsg blocks until some frame the engine has fanned out to conn satisfies pred.
//
// It carries no deadline and no poll interval. The engine calls WriteMessage every time it
// emits a frame, so that call is the real signal, and conn.notify publishes it; between
// edges there is by definition nothing new to test. A predicate that is never satisfied
// means the engine never produced the output the test was waiting for — a hang, which
// `go test -timeout` reports with the full goroutine dump.
//
// The pred is evaluated against the ACCUMULATED inbox rather than the single newest frame,
// so a payload split across frames still matches.
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
			// A new frame landed; re-scan.
		case <-conn.closed:
			// The conn is closed: no further frame can arrive. Re-check once for a frame
			// that landed in the same instant, then fail rather than block forever.
			if connMatches(conn, pred) {
				return
			}
			t.Fatal("conn closed before the awaited frame arrived")
		}
	}
}

// connMatches reports whether any frame received so far satisfies pred.
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
	}) // blocks on the fan-out signal: "must receive output containing 'hello'"

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

	// Kill the underlying PTY process to make the session dead,
	// then try to attach — should get an error.
	require.NoError(t, eng.Kill(ctx, sid))

	// Re-inject the dead session into registry to test Attach on dead session.
	// We cannot do this from outside without access to internals, so instead
	// we verify Attach returns error for a non-existent session ID.
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

	// Wait until the seqConn has delivered all messages.
	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-conn.allConsumed

	_ = conn.Close()
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

	// Write to trigger writePump which will fail on WriteMessage.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo hi\n")))

	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-attachDone
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
	// A persisted blob is a CRWB1 header + redraw bytes (§12). The body prints the marker,
	// so after restore→attach the serialized screen reproduces it.
	scrollback := []byte("CRWB1 80 24 0 10000\n$ echo " + marker + "\r\n" + marker + "\r\n")

	// Write the blob to disk so restore() can read it.
	// (restore re-reads from disk, not from the placeholder.)
	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-ph-attach",
		CWD:       dir,
		Shell:     "/bin/sh",
		ProfileID: "",
	}, scrollback))

	// Attach must trigger restore transparently.
	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// The replayed scrollback must appear in the first messages.
	waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, marker)
	}) // blocks on the fan-out signal: "scrollback must be replayed after LoadPlaceholder → Attach"

	_ = conn.Close()
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
	// blocks on the fan-out signal — must receive 'detach-marker' output before detaching
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

// TestEngine_Suspend_WithConnectedClient_NoOp verifies that Suspend is a no-op
// when a client is currently attached (BeginSuspendIfEligible returns false).
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

	// Wait for at least one frame to confirm the client is fully registered.
	// blocks on the fan-out signal — must receive initial PTY output to confirm attach is live
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	// Suspend with a connected client must be a no-op.
	require.NoError(t, eng.Suspend(ctx, sid))
	assert.True(t, eng.SessionExists(ctx, sid), "session must still exist after no-op suspend")

	// Session must still be writable (live PTY).
	assert.NoError(t, eng.Write(ctx, sid, []byte("echo still-alive\n")))

	_ = conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_Suspend_MakesPlaceholder verifies that after Suspend on an idle session:
//   - the session still exists in the registry (as a placeholder)
//   - a .buf file is written to disk
//   - meta is saved with state="suspended"
//   - its reaper does not clean the .buf and row up as if the session had ended
func TestEngine_Suspend_MakesPlaceholder(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	// The shell is parked at its prompt, hence idle, which is the precondition Suspend's
	// eligibility gate checks. So ONE call must take — no retry loop.
	sid := newReadyShell(t, eng, "chat-susp", dir)
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	assert.True(t, eng.SessionExists(ctx, sid), "suspended session must still be in registry")
	assert.True(t, bufExists(store.dir, sid), ".buf must exist after suspend")

	// The killed process's reaper must not clean up as if the session ended; Shutdown
	// joins it, so what survives afterwards is final.
	eng.Shutdown()
	assert.True(t, bufExists(store.dir, sid), ".buf must survive the suspended process's reap")
	assert.True(t, store.hasLiveRow(sid), "the suspended row must survive the reap")
}

// TestEngine_Suspend_ThenAttach_Restores verifies that attaching to a suspended
// session triggers restore: a fresh shell spawns, prior scrollback is replayed,
// and new output flows normally.
func TestEngine_Suspend_ThenAttach_Restores(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-restore", dir)

	// Attach first client, write something to populate the ring, then detach.
	conn1 := newMockConn()
	attach1Done := make(chan struct{})
	go func() {
		defer close(attach1Done)
		_ = eng.Attach(ctx, sid, conn1)
	}()
	require.NoError(t, eng.Write(ctx, sid, []byte("echo pre-suspend\n")))
	waitForMsg(t, conn1, func(d string) bool { return containsStr(d, "pre-suspend") })
	_ = conn1.Close()

	// Join the Attach goroutine. Attach runs the client's read pump and calls Detach on its
	// way out, so its RETURN is the detach-bookkeeping completing — the real signal that the
	// session has no attached clients left, which is exactly the precondition Suspend needs.
	// Polling StateOf for "detached" was watching for the shadow of this event.
	<-attach1Done
	st, ok := eng.StateOf(sid)
	require.True(t, ok)
	require.Equal(t, "detached", st, "the session must be detached once its last client has gone")

	// Idle (at its prompt) and detached: Suspend must take on the first call.
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	// Attach to the suspended session → restore must happen transparently.
	conn2 := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn2)
	}()

	// Restored session must replay prior scrollback.
	waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "pre-suspend")
	}) // blocks on the fan-out signal: "scrollback must be replayed after restore"

	// New output must flow through the restored live shell.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo post-restore\n")))
	waitForMsg(t, conn2, func(d string) bool {
		return containsStr(d, "post-restore")
	}) // blocks on the fan-out signal: "new output must flow after restore"

	_ = conn2.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_Kill_CleanReap verifies that Kill triggers a full reap: session removed
// from registry, meta deleted, .buf deleted.
func TestEngine_Kill_CleanReap(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "chat-kill", dir, nil)
	require.NoError(t, err)

	// Pre-write a .buf so we can verify Kill deletes it.
	bufFile := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufFile, []byte("scrollback"), 0o644))

	require.NoError(t, eng.Kill(ctx, sid))
	// Shutdown joins the reaper, so its cleanup has run by the assertions below.
	eng.Shutdown()

	assert.False(t, eng.SessionExists(ctx, sid), "session must be absent after kill")
	assert.True(t, store.hasDeleted(sid), "meta must be deleted by reapOnDone")
	assert.False(t, bufExists(store.dir, sid), ".buf must be deleted by reapOnDone")
}
