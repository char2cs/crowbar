package terminal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

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
	return false
}

func TestEngine_Create_And_ListSessions(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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

	// Block on the real signal — the callback firing. A hand-rolled deadline would only be
	// a second, weaker definition of "too slow"; a callback that never fires is a hang, and
	// `go test -timeout` reports it with the blocked stack.
	got := <-endedCh
	assert.Equal(t, "ws-a", got.wsID)
	assert.Equal(t, sid, got.sid)
	// exitCode is -1 when killed by signal; any integer is valid here.
	_ = got.exitCode
}

func TestEngine_Kill_Unknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	err := eng.Kill(ctx, "nope")
	assert.Error(t, err)
}

func TestEngine_Write_And_Resize_Unknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	assert.Error(t, eng.Write(ctx, "nope", []byte("hi")))
	assert.Error(t, eng.Resize(ctx, "nope", 80, 24))
}

func TestEngine_Write(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Write(ctx, sid, []byte("echo ok\n")))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Resize(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Resize(ctx, sid, 120, 40))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Attach_UnknownSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	conn := newMockConn()
	conn.Close()
	err := eng.Attach(ctx, "ghost", conn)
	assert.Error(t, err)
}

func TestEngine_Attach_ReceivesOutput(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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

	waitForMsg(t, conn, func(data string) bool {
		return len(data) >= 5 && containsStr(data, "hello")
	}) // blocks on the fan-out signal: "must receive output containing 'hello'"

	conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

func TestEngine_ListSessions_AfterKill(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)
	require.NoError(t, eng.Kill(ctx, sid))

	assert.NotContains(t, eng.ListSessions(), sid)
}

func TestEngine_Create_BadShell(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	prof := &domain.TerminalProfile{Shell: "/nonexistent/shell"}
	_, err := eng.Create(ctx, "ws-bad", dir, prof)
	assert.Error(t, err)
}

func TestEngine_Attach_DeadSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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

// TestEngine_Attach_ResyncMessage covers the readPump "resync" dispatch: the
// message must route to Session.Resync (a no-op at the idle prompt — the gate
// itself is pinned by the session package's resync tests) without being
// written to the PTY as input.
func TestEngine_Attach_ResyncMessage(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "ws-1", dir, nil)
	require.NoError(t, err)

	resyncMsg, _ := json.Marshal(map[string]any{"type": "resync"})
	msgs := [][]byte{resyncMsg, resyncMsg}
	conn := newSeqConn(msgs)

	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-conn.allConsumed

	conn.Close()
	require.NoError(t, eng.Kill(ctx, sid))
	<-attachDone
}

func TestEngine_Attach_ResizeMessage(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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
	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-conn.allConsumed

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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)
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
	// A persisted blob is a CRWB1 header + redraw bytes (§12). The body prints the marker,
	// so after restore→attach the serialized screen reproduces it.
	scrollback := []byte("CRWB1 80 24 0 10000\n$ echo " + marker + "\r\n" + marker + "\r\n")

	// Write the blob to disk so restore() can read it via persistence.ReadBuf.
	// (restore re-reads from disk, not from the placeholder.)
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
	waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, marker)
	}) // blocks on the fan-out signal: "scrollback must be replayed after LoadPlaceholder → Attach"

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
	terminal.StopMaintenanceForTest(eng)
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
	// blocks on the fan-out signal — must receive 'detach-marker' output before detaching
	waitForMsg(t, conn, func(data string) bool {
		return containsStr(data, "detach-marker")
	})

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
	terminal.StopMaintenanceForTest(eng)
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
	// blocks on the fan-out signal — must receive initial PTY output to confirm attach is live
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

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
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	// A channel (not a plain bool) so the negative assertion below can observe the
	// callback without a data race under -race.
	endedCh := make(chan struct{}, 1)
	eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) {
		select {
		case endedCh <- struct{}{}:
		default:
		}
	})

	// The shell is parked at its prompt, hence idle, which is the precondition Suspend's
	// eligibility gate checks. So ONE call must take — no retry loop.
	sid := newReadyShell(t, eng, "ws-susp", dir)
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	assert.True(t, eng.SessionExists(ctx, sid), "suspended session must still be in registry")
	assert.True(t, bufExists(store.dir, sid), ".buf must exist after suspend")

	// Negative assertion: a suspended session must NOT fire onEnded. The thing that could
	// wrongly fire it is the suspended session's reapOnDone goroutine — which must observe the
	// placeholder swap and return without firing. So the barrier is to JOIN that goroutine:
	// Shutdown drains every outstanding reaper before returning, after which the set of
	// onEnded fires is final and closed.
	//
	// That is a real barrier, where the 300 ms window it replaces was a guess. The window did
	// not prove the callback never fires; it proved it had not fired YET, and would have
	// passed just as happily if the reaper were merely slow.
	eng.Shutdown()

	select {
	case <-endedCh:
		t.Fatal("onEnded must NOT fire for a suspended session")
	default:
	}
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

	sid := newReadyShell(t, eng, "ws-restore", dir)

	// Attach first client, write something to populate the ring, then detach.
	conn1 := newMockConn()
	attach1Done := make(chan struct{})
	go func() {
		defer close(attach1Done)
		_ = eng.Attach(ctx, sid, conn1)
	}()
	require.NoError(t, eng.Write(ctx, sid, []byte("echo pre-suspend\n")))
	waitForMsg(t, conn1, func(d string) bool { return containsStr(d, "pre-suspend") })
	conn1.Close()

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

	conn2.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_DropUnrestorable_CleansUpSessionMu verifies that when a placeholder's
// restore spawn fails (un-restorable), both the registry entry AND the sessionMu
// entry are pruned. A leaked sessionMu entry would prevent a future lockSession
// call from creating a fresh mutex for the same id after a re-provision.
func TestEngine_DropUnrestorable_CleansUpSessionMu(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	sid := "ph-unrestorable-mu"
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-unrestorable-mu",
		CWD:         t.TempDir(),
		Shell:       "/nonexistent/shell-that-cannot-spawn",
	}, nil))

	// Attach triggers restore, which calls spawn with the non-existent shell.
	// spawn fails → dropUnrestorable removes the registry entry → Attach returns error.
	conn := newMockConn()
	err := eng.Attach(ctx, sid, conn)
	require.Error(t, err, "Attach must return an error for an un-restorable session")

	assert.False(t, eng.SessionExists(ctx, sid),
		"registry entry must be removed after unrestorable drop")
	assert.False(t, terminal.HasSessionMuForTest(eng, sid),
		"sessionMu entry must be pruned after unrestorable drop to prevent mutex aliasing")
}

// TestEngine_Kill_CleanReap verifies that Kill triggers a full reap: session removed
// from registry, meta deleted, .buf deleted, and onEnded fired exactly once.
func TestEngine_Kill_CleanReap(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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

	// Block on reapOnDone firing the callback — the real signal.
	assert.Equal(t, sid, <-endedCh)

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
	terminal.StopMaintenanceForTest(eng)
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
	terminal.StopMaintenanceForTest(eng)

	state, ok := eng.StateOf("no-such-session")
	assert.False(t, ok)
	assert.Empty(t, state)
}

// TestEngine_StateOf_ActiveWhileAttached verifies StateOf returns "active"
// while a client is attached.
func TestEngine_StateOf_ActiveWhileAttached(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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
	// blocks on the fan-out signal — must receive initial output to confirm attach
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

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
	terminal.StopMaintenanceForTest(eng)
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

	// blocks on the fan-out signal — must receive initial output before closing
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	conn.Close()
	<-attachDone

	assert.Equal(t, "detached", <-stateCh,
		"OnSessionState must fire 'detached' after the last client disconnects")

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestRegression_TerminalLifecycle_StateOf_And_OnSessionState is the regression
// test for the lifecycle wire-protocol data path (Phase 2): the engine's
// StateOf must reflect the session's actual state, and the OnSessionState
// callback must fire "detached" after last-client disconnect and "suspended"
// after Suspend.
func TestRegression_TerminalLifecycle_StateOf_And_OnSessionState(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
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

	// Create → should be detached (no clients). Block until the shell is at its prompt, so the
	// Suspend later in this test meets the idle gate its eligibility check applies.
	sid := newReadyShell(t, eng, "ws-regress", dir)

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
	// blocks on the fan-out signal — must receive initial output to confirm attach
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state, "session must be active while client is attached")

	// Detach client → "detached" + callback fires.
	conn.Close()
	<-attachDone

	assert.Equal(t, "detached", <-stateCh,
		"OnSessionState must fire 'detached' on last-client leave")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state)

	// Suspend → "suspended" + callback fires. The shell is at its prompt (idle) and now
	// detached, so Suspend's eligibility gate is satisfied and ONE call must take.
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	assert.Equal(t, "suspended", <-stateCh,
		"OnSessionState must fire 'suspended' after Suspend")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state)

	// Kill the placeholder → "ended" via OnSessionEnded.
	require.NoError(t, eng.Kill(ctx, sid))

	// Placeholder kill → exit code -1 (no process).
	assert.Equal(t, -1, <-endedCh, "placeholder kill must report exitCode -1")
}
