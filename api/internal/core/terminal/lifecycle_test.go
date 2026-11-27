package terminal_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

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

func TestRestore_CWDFallback_SpawnsInHomeWhenSavedDirGone(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	goneDir := filepath.Join(t.TempDir(), "deleted-worktree")
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: "ph-cwd", ChatID: "chat-1", CWD: goneDir, Shell: "/bin/sh",
	}, nil))

	require.NoError(t, eng.Attach(ctx, "ph-cwd", newClosingConn()),
		"restore must succeed via the CWD fallback")
	assert.True(t, eng.SessionLive(ctx, "ph-cwd"), "restored session must be live")
	_ = eng.Kill(ctx, "ph-cwd")
}

func TestRestore_BadShell_Removes(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	require.NoError(t, os.WriteFile(filepath.Join(store.dir, "ph-bad.buf"), []byte("x"), 0o644))
	require.NoError(t, store.Save(ctx, terminal.SessionMeta{SessionID: "ph-bad", State: "suspended"}))
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: "ph-bad", ChatID: "chat-1", CWD: t.TempDir(), Shell: "/nonexistent/shell/binary",
	}, nil))

	require.Error(t, eng.Attach(ctx, "ph-bad", newClosingConn()))
	assert.False(t, eng.SessionExists(ctx, "ph-bad"), "no infinite Attach retry")
	assert.False(t, bufExists(store.dir, "ph-bad"), "persisted .buf must be deleted")
	assert.False(t, store.hasLiveRow("ph-bad"), "meta row must be deleted")
}

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
		<-c.closed
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
	require.NoError(t, conn.Close())
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
	terminal.RunMaintenanceOnceForTest(eng, ctx)
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

type degradedModel struct{ cols, rows int }

func (m *degradedModel) Write([]byte)                       {}
func (m *degradedModel) Resize(c, r int)                    { m.cols, m.rows = c, r }
func (m *degradedModel) OnForegroundReset()                 {}
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

func TestTerminate_ACommandSessionEndsWithoutTouchingStorage(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	ctx := context.Background()
	sid, err := eng.CreateCommand(ctx, "chat-erased", t.TempDir(), []string{"/bin/sh", "-c", "sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)

	require.NoError(t, eng.TerminateGraceful(ctx, sid))

	assert.False(t, store.hasDeleted(sid))
}
