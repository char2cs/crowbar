package terminal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// blockingConn is a WSConn whose ReadMessage blocks until Close, so an Attach stays attached
// (readPump parked) until the test releases it. wrote is a coalescing edge published on
// every frame the engine sends: Attach's first act is the snapshot keyframe, so the first
// edge proves the client is registered.
type blockingConn struct {
	closed chan struct{}
	once   sync.Once
	wrote  chan struct{}
}

func newBlockingConn() *blockingConn {
	return &blockingConn{closed: make(chan struct{}), wrote: make(chan struct{}, 1)}
}

func (c *blockingConn) waitAttached() { <-c.wrote }

func (c *blockingConn) WriteMessage(_ int, _ []byte) error {
	select {
	case c.wrote <- struct{}{}:
	default:
	}
	return nil
}
func (c *blockingConn) ReadMessage() (int, []byte, error) { <-c.closed; return 0, nil, io.EOF }
func (c *blockingConn) SetWriteDeadline(time.Time) error  { return nil }
func (c *blockingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// TestRestore_CWDFallback_SpawnsInHomeWhenSavedDirGone: a saved working directory that no
// longer exists must not strand a suspended session — the restore falls back and succeeds.
func TestRestore_CWDFallback_SpawnsInHomeWhenSavedDirGone(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	goneDir := filepath.Join(t.TempDir(), "deleted-worktree") // never created
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: "ph-cwd", ChatID: "chat-1", CWD: goneDir, Shell: "/bin/sh",
	}, nil))

	require.NoError(t, eng.Attach(ctx, "ph-cwd", newClosingConn()),
		"restore must succeed via the CWD fallback")
	assert.True(t, eng.SessionLive(ctx, "ph-cwd"), "restored session must be live")
	_ = eng.Kill(ctx, "ph-cwd")
}

// TestRestore_BadShell_RemovesAndReportsEnded: a suspended session that cannot be
// re-spawned (its shell binary is gone) is Removed — registry entry, .buf and meta row all
// deleted, ended fired once — so an Attach can never loop forever on a doomed restore.
func TestRestore_BadShell_RemovesAndReportsEnded(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	ledger := newEndedLedger()
	eng.OnSessionEnded(ledger.onEnded)

	require.NoError(t, os.WriteFile(filepath.Join(store.dir, "ph-bad.buf"), []byte("x"), 0o644))
	require.NoError(t, store.Save(ctx, terminal.SessionMeta{SessionID: "ph-bad", State: "suspended"}))
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: "ph-bad", ChatID: "chat-1", CWD: t.TempDir(), Shell: "/nonexistent/shell/binary",
	}, nil))

	require.Error(t, eng.Attach(ctx, "ph-bad", newClosingConn()))
	assert.False(t, eng.SessionExists(ctx, "ph-bad"), "no infinite Attach retry")
	assert.False(t, bufExists(store.dir, "ph-bad"), "persisted .buf must be deleted")
	assert.False(t, store.hasLiveRow("ph-bad"), "meta row must be deleted")
	ended, _ := ledger.counts("ph-bad")
	assert.Equal(t, 1, ended, "ended must fire once so the FE drops the dead tab")
}

// TestAttach_ExitFrameOnSelfExit pins B4: a PTY exit reaches the attached client as an
// explicit exit frame, carrying the exit code, before the conn closes.
func TestAttach_ExitFrameOnSelfExit(t *testing.T) {
	pinShell(t)
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	sid := newReadyShell(t, eng, "chat-exit", t.TempDir())

	conn := newMockConn()
	attached := make(chan error, 1)
	go func() { attached <- eng.Attach(ctx, sid, conn) }()
	waitForMsg(t, conn, func(string) bool { return true })

	require.NoError(t, eng.Write(ctx, sid, []byte("exit 3\n")))
	require.NoError(t, <-attached)
	code, ok := exitFrameCode(conn)
	require.True(t, ok, "the client must receive an exit frame")
	assert.Equal(t, 3, code)
}

// TestShutdown_NoExitFrameForPersistedShell: Shutdown persists a live shell for the next
// boot — it did not exit, so its client must NOT be told it did (it reconnects instead).
func TestShutdown_NoExitFrameForPersistedShell(t *testing.T) {
	pinShell(t)
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	sid := newReadyShell(t, eng, "chat-sd", store.dir)

	conn := newMockConn()
	attached := make(chan error, 1)
	go func() { attached <- eng.Attach(ctx, sid, conn) }()
	waitForMsg(t, conn, func(string) bool { return true })

	eng.Shutdown()
	<-attached
	_, ok := exitFrameCode(conn)
	assert.False(t, ok, "a session persisted for restart must not report an exit")
}

// TestAttach_BoundedWhenClientStalls pins B6: a client that stops reading costs at most the
// write deadline — Attach returns instead of parking forever behind a blocked write.
func TestAttach_BoundedWhenClientStalls(t *testing.T) {
	pinShell(t)
	eng := terminal.NewWithWriteWaitForTest(50 * time.Millisecond)
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	sid := newReadyShell(t, eng, "chat-stall", t.TempDir())

	conn := newStallConn()
	attached := make(chan error, 1)
	go func() { attached <- eng.Attach(ctx, sid, conn) }()
	select {
	case <-attached:
	case <-time.After(5 * time.Second):
		t.Fatal("Attach must return once its stalled client's write deadline passes")
	}
}

// stallConn never completes a write until its deadline passes, and never sends input.
type stallConn struct {
	mu       sync.Mutex
	deadline time.Time
	closed   chan struct{}
	once     sync.Once
}

func newStallConn() *stallConn { return &stallConn{closed: make(chan struct{})} }

func (c *stallConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return nil
}

func (c *stallConn) WriteMessage(int, []byte) error {
	c.mu.Lock()
	d := c.deadline
	c.mu.Unlock()
	if d.IsZero() {
		<-c.closed // a write with no deadline blocks for ever: the bug under test
		return io.ErrClosedPipe
	}
	select {
	case <-time.After(time.Until(d)):
		return os.ErrDeadlineExceeded
	case <-c.closed:
		return io.ErrClosedPipe
	}
}

func (c *stallConn) ReadMessage() (int, []byte, error) { <-c.closed; return 0, nil, io.EOF }
func (c *stallConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// TestKill_SignalsProcessGroup: Kill must take the session's children down with it, as
// Terminate does — a background child of a command session must not survive it.
func TestKill_SignalsProcessGroup(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	id, err := eng.CreateCommand(ctx, "chat-pg", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 300 & echo $! > " + pidFile + "; wait"}, nil, nil)
	require.NoError(t, err)

	var pid int
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil || len(b) == 0 {
			return false
		}
		_, scanErr := fmtSscan(string(b), &pid)
		return scanErr == nil && pid > 0
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, eng.Kill(ctx, id))
	require.Eventually(t, func() bool { return !processAlive(pid) }, 5*time.Second, 10*time.Millisecond,
		"the session's background child must die with it")
}

// ---------------------------------------------------------------------------
// Maintenance ceiling
// ---------------------------------------------------------------------------

func TestMaintenance_AttachedSessionSkipped(t *testing.T) {
	pinShell(t)
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	restore := terminal.SetSoftLimitPerChatForTest(eng, 0)
	defer restore()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-att", store.dir)
	conn := newBlockingConn()
	attachReturned := make(chan struct{})
	go func() {
		_ = eng.Attach(ctx, sid, conn)
		close(attachReturned)
	}()
	conn.waitAttached()
	st, _ := eng.StateOf(sid)
	require.Equal(t, "active", st)
	active, _, _, _, _, _ := eng.Stats()
	assert.GreaterOrEqual(t, active, 1)

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	st, _ = eng.StateOf(sid)
	assert.Equal(t, "active", st, "an attached session must never be suspended by maintenance")
	conn.Close()
	<-attachReturned
}

func TestMaintenance_GlobalCeilingSuspendsOldestIdle(t *testing.T) {
	pinShell(t)
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid1 := newReadyShell(t, eng, "chat-3a", store.dir)
	sid2 := newReadyShell(t, eng, "chat-3a", store.dir)
	// A ceiling at 3/4 of two fresh grids: cache drops cannot get under, one suspend can.
	_, _, _, fresh, _, _ := eng.Stats()
	restore := terminal.SetMaxTotalModelBytesForTest(eng, fresh*3/4)
	defer restore()
	base := time.Now().Add(-time.Hour)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	st1, _ := eng.StateOf(sid1)
	st2, _ := eng.StateOf(sid2)
	assert.Equal(t, "suspended", st1, "the oldest idle session is suspended first")
	assert.Equal(t, "detached", st2, "the newer session stays live")
}

func TestMaintenance_CacheDropResolvesCeiling(t *testing.T) {
	pinShell(t)
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sids := []string{newReadyShell(t, eng, "chat-cache", store.dir), newReadyShell(t, eng, "chat-cache", store.dir)}
	terminal.RunMaintenanceOnceForTest(eng, ctx) // populate each session's cached blob
	_, _, _, total, _, _ := eng.Stats()
	restore := terminal.SetMaxTotalModelBytesForTest(eng, total-1)
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	restore()
	for _, sid := range sids {
		st, _ := eng.StateOf(sid)
		assert.Equal(t, "detached", st, "cache reclaim alone must resolve the ceiling")
	}
}

func TestMaintenanceLoop_TickerFlushes(t *testing.T) {
	pinShell(t)
	eng := terminal.NewWithTickForTest(5 * time.Millisecond)
	defer eng.Shutdown()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	sid := newReadyShell(t, eng, "chat-tick", store.dir)
	require.Eventually(t, func() bool { return bufExists(store.dir, sid) }, 5*time.Second, 5*time.Millisecond,
		"the maintenance ticker must drive a cadence flush")
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

type degradedModel struct{ cols, rows int }

func (m *degradedModel) Write([]byte)                       {}
func (m *degradedModel) Resize(c, r int)                    { m.cols, m.rows = c, r }
func (m *degradedModel) OnForegroundReset()                 {}
func (m *degradedModel) PendingInput() []byte               { return nil }
func (m *degradedModel) Title() string                      { return "" }
func (m *degradedModel) Cols() int                          { return m.cols }
func (m *degradedModel) Rows() int                          { return m.rows }
func (m *degradedModel) HeaderState() (int, int, bool, int) { return m.cols, m.rows, false, 0 }
func (m *degradedModel) ModelBytes() int64                  { return 0 }
func (m *degradedModel) Close()                             {}
func (m *degradedModel) SetResponseSink(func(p []byte))     {}
func (m *degradedModel) Degraded() bool                     { return true }
func (m *degradedModel) ParsePanics() int                   { return 7 }

type degradedSerializer struct{}

func (degradedSerializer) Serialize(model.TerminalModel) []byte { return []byte("X") }

func TestStats_CountsDegraded(t *testing.T) {
	pinShell(t)
	restore := session.SetNewModelForTest(func(c, r, _ int) (model.TerminalModel, model.Serializer) {
		return &degradedModel{cols: c, rows: r}, degradedSerializer{}
	})
	defer restore()
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()

	_, err := eng.Create(context.Background(), "chat-deg", t.TempDir(), nil)
	require.NoError(t, err)
	_, _, _, _, degraded, parsePanics := eng.Stats()
	assert.GreaterOrEqual(t, degraded, 1)
	assert.GreaterOrEqual(t, parsePanics, 7)
}

// exitFrameCode scans conn's received frames for the engine's exit frame.
func exitFrameCode(conn *mockConn) (int, bool) {
	for _, raw := range conn.allReceived() {
		var msg struct {
			Type string `json:"type"`
			Code int    `json:"code"`
		}
		if json.Unmarshal(raw, &msg) == nil && msg.Type == "exit" {
			return msg.Code, true
		}
	}
	return 0, false
}

func fmtSscan(s string, pid *int) (int, error) { return fmt.Sscan(strings.TrimSpace(s), pid) }

// processAlive reports whether pid is a running (non-zombie) process. A container's init
// may never reap an orphan, so a dead child can linger as a zombie; that counts as dead.
func processAlive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return syscall.Kill(pid, 0) == nil // no procfs (darwin): best effort
	}
	fields := strings.Fields(string(b))
	return len(fields) > 2 && fields[2] != "Z"
}
