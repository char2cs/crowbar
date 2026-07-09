package terminal

// Internal (package terminal) hardening tests. They drive the engine's defensive and
// race-guard branches that the external test package cannot reach, using narrow seams: a
// hooked sessionRegistry (lookup-race guards), the session model seam (degraded Stats), the
// done-closed session seam (live-Attach error), the startupWrite seam (Create), the
// maintenance-interval seam (ticker branch), and deterministic filesystem failures
// (WriteBuf/DeleteBuf error logging).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

// ---------------------------------------------------------------------------
// Shared test doubles
// ---------------------------------------------------------------------------

// coverStore is a SessionMetaStore whose StorageDir is fully controllable so a test can point
// it at a regular file (forcing WriteBuf to fail) or a real directory.
type coverStore struct {
	mu      sync.Mutex
	dir     string
	saved   []SessionMeta
	deleted []string
}

func (s *coverStore) Save(_ context.Context, m SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, m)
	return nil
}

func (s *coverStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *coverStore) StorageDir(_ context.Context, _ string) (string, error) {
	return s.dir, nil
}

func (s *coverStore) List(_ context.Context) ([]domain.TerminalSession, error) { return nil, nil }

func (s *coverStore) savedState(id, state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.saved {
		if m.SessionID == id && m.State == state {
			return true
		}
	}
	return false
}

// blockingConn is a WSConn whose ReadMessage blocks until Close, so an Attach stays attached
// (readPump parked) until the test releases it.
type blockingConn struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingConn() *blockingConn { return &blockingConn{closed: make(chan struct{})} }

func (c *blockingConn) WriteMessage(_ int, _ []byte) error { return nil }
func (c *blockingConn) ReadMessage() (int, []byte, error)  { <-c.closed; return 0, nil, io.EOF }
func (c *blockingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// hookedReg wraps a real *registry.Registry, optionally overriding Get/WorkspaceID/List so a
// test can simulate a session vanishing from the map between two reads.
type hookedReg struct {
	*registry.Registry
	getFn  func(id string, base *registry.Registry) (*session.Session, bool)
	wsFn   func(id string, base *registry.Registry) (string, bool)
	listFn func(base *registry.Registry) []string
}

func (h *hookedReg) Get(id string) (*session.Session, bool) {
	if h.getFn != nil {
		return h.getFn(id, h.Registry)
	}
	return h.Registry.Get(id)
}

func (h *hookedReg) WorkspaceID(id string) (string, bool) {
	if h.wsFn != nil {
		return h.wsFn(id, h.Registry)
	}
	return h.Registry.WorkspaceID(id)
}

func (h *hookedReg) List() []string {
	if h.listFn != nil {
		return h.listFn(h.Registry)
	}
	return h.Registry.List()
}

// degradedModel is an inert model that always reports a degraded parse-health surface, so the
// engine's Stats aggregation has a degraded session to count without a real vt parse panic.
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

var (
	_ model.TerminalModel = (*degradedModel)(nil)
	_ model.ModelHealth   = (*degradedModel)(nil)
	_ model.Serializer    = degradedSerializer{}
)

// newCoverEngine builds an engine with the maintenance ticker stopped, returning it and its
// underlying real registry (for tests that wrap it in a hookedReg).
func newCoverEngine(t *testing.T) (*terminalEngine, *registry.Registry) {
	t.Helper()
	e := New().(*terminalEngine)
	StopMaintenanceForTest(e)
	real := e.reg.(*registry.Registry)
	return e, real
}

// makeFilePath returns a path that is an existing regular file, so persistence.WriteBuf's
// MkdirAll fails and the write errors deterministically.
func makeFilePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	return p
}

// makeBadBufDir creates <dir>/<id>.buf as a NON-EMPTY directory, so persistence.DeleteBuf's
// os.Remove fails with a real (non-IsNotExist) error.
func makeBadBufDir(t *testing.T, dir, id string) {
	t.Helper()
	bad := filepath.Join(dir, id+".buf")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "blocker"), []byte("y"), 0o644))
}

// ---------------------------------------------------------------------------
// Simple / pure-function gaps
// ---------------------------------------------------------------------------

func TestMaxTotalSessions_ReturnsCeiling(t *testing.T) {
	assert.Equal(t, maxTotalSessions, MaxTotalSessions(),
		"MaxTotalSessions must return the package-level global ceiling")
}

func TestDirExists(t *testing.T) {
	assert.False(t, dirExists(""), "empty path is not a directory")
	d := t.TempDir()
	assert.True(t, dirExists(d), "a real directory must report true")
	f := makeFilePath(t)
	assert.False(t, dirExists(f), "a regular file is not a directory")
}

func TestResolveRestoreCWD(t *testing.T) {
	// Existing dir → returned verbatim, no notice.
	d := t.TempDir()
	cwd, notice := resolveRestoreCWD(d)
	assert.Equal(t, d, cwd)
	assert.Nil(t, notice)

	// Missing dir with HOME unset → home resolves to "" and a notice is produced.
	t.Setenv("HOME", "")
	cwd, notice = resolveRestoreCWD(filepath.Join(d, "gone", "missing"))
	assert.Equal(t, "", cwd, "an unresolvable HOME must fall back to the empty string")
	assert.NotEmpty(t, notice, "a CWD fallback must produce an on-screen notice")
}

func TestTrailingIncompleteUTF8(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want int
	}{
		{"empty", nil, 0},
		{"ascii", []byte("abc"), 0},
		{"single-ascii", []byte("a"), 0},
		{"complete-2byte", []byte{0xC3, 0xA9}, 0},             // é, full
		{"lone-2byte-lead", []byte{0xC3}, 1},                  // lead present, 1 cont missing
		{"truncated-3byte", []byte{0xE2, 0x82}, 2},            // 3-byte rune missing 1 cont
		{"truncated-4byte", []byte{0xF0, 0x9F, 0x98}, 3},      // 4-byte rune missing 1 cont
		{"orphan-continuations", []byte{0x80, 0x80, 0x80}, 0}, // no lead within last 3
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, trailingIncompleteUTF8(c.in))
		})
	}
}

// ---------------------------------------------------------------------------
// writePump branches
// ---------------------------------------------------------------------------

// TestWritePump_ClosedDuringDrain covers the drain-coalesce, channel-closed-mid-drain, and
// closed-return branches: two pre-queued frames are coalesced by the inner drain (the first is
// appended), and the subsequent close makes the drain observe !ok.
func TestWritePump_ClosedDuringDrain(t *testing.T) {
	e, _ := newCoverEngine(t)
	conn := newBlockingConn()
	ch := make(chan session.OutputFrame, 4)
	done := make(chan struct{})

	// Two frames are buffered before writePump runs: the first range iteration drains the
	// second into the same message (the coalesce-append path), then sees the close (!ok).
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("hello ")}
	ch <- session.OutputFrame{SessionID: "s", Data: []byte("world")}
	close(ch) // the drain's <-ch will observe the close (ok==false)

	go e.writePump(conn, "s", ch, done)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writePump did not return after the channel closed")
	}
}

// recordConn records every written message so the holdback split is observable.
type recordConn struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (c *recordConn) WriteMessage(_ int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := append([]byte(nil), data...)
	c.msgs = append(c.msgs, cp)
	return nil
}
func (c *recordConn) ReadMessage() (int, []byte, error) { select {} }
func (c *recordConn) Close() error                      { return nil }
func (c *recordConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

// TestWritePump_HoldbackSplitObservable proves the holdback actually splits the stream: the
// first emitted message excludes the dangling lead byte, and the rune is delivered whole in a
// later message.
func TestWritePump_HoldbackSplitObservable(t *testing.T) {
	e, _ := newCoverEngine(t)
	conn := &recordConn{}
	ch := make(chan session.OutputFrame, 1)
	done := make(chan struct{})
	go e.writePump(conn, "s", ch, done)

	dataOf := func(raw []byte) string {
		var m struct {
			Data string `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &m))
		return m.Data
	}

	ch <- session.OutputFrame{SessionID: "s", Data: []byte{'h', 'i', 0xF0}}
	require.Eventually(t, func() bool { return conn.count() >= 1 }, time.Second, 5*time.Millisecond)

	conn.mu.Lock()
	first := dataOf(conn.msgs[0])
	conn.mu.Unlock()
	assert.Equal(t, "hi", first, "the first message's data must withhold the dangling lead byte")

	ch <- session.OutputFrame{SessionID: "s", Data: []byte{0x9F, 0x98, 0x81}}
	close(ch)
	<-done

	conn.mu.Lock()
	defer conn.mu.Unlock()
	require.GreaterOrEqual(t, len(conn.msgs), 2, "the completed rune must arrive in a later message")
	assert.Contains(t, dataOf(conn.msgs[1]), "\xF0\x9F\x98\x81",
		"the second message's data must carry the now-complete 4-byte rune")
}

// ---------------------------------------------------------------------------
// restore / Create / Attach error guards
// ---------------------------------------------------------------------------

// TestRestore_SessionGone covers restore's not-found early return: a restore of an id that is
// not in the registry returns nil (the session was concurrently killed).
func TestRestore_SessionGone(t *testing.T) {
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	assert.NoError(t, e.restore(context.Background(), "never-existed"),
		"restore of a missing session must be a no-op")
}

// TestCreate_StartupWriteError covers the startup-command write-failure break: a failed
// startup write is non-fatal (the PTY is alive) and Create still succeeds.
func TestCreate_StartupWriteError(t *testing.T) {
	e, _ := newCoverEngine(t)
	defer e.Shutdown()

	orig := startupWrite
	startupWrite = func(*session.Session, []byte) error { return errors.New("startup write boom") }
	defer func() { startupWrite = orig }()

	prof := &domain.TerminalProfile{Shell: "/bin/sh", StartupCommands: []string{"echo one", "echo two"}}
	sid, err := e.Create(context.Background(), "ws-startup", t.TempDir(), prof)
	require.NoError(t, err, "a startup-write failure must NOT fail Create (non-fatal)")
	assert.NotEmpty(t, sid)
	_ = e.Kill(context.Background(), sid)
}

// TestAttach_LiveSessionAttachError covers the s.Attach() error path inside engine Attach: a
// session that reports IsLive()==true but whose own Attach fails (done already closed) must
// surface an error without restoring.
func TestAttach_LiveSessionAttachError(t *testing.T) {
	e, real := newCoverEngine(t)
	defer e.Shutdown()

	s := session.NewDoneClosedForTest("done-closed", "/bin/sh", t.TempDir(), "")
	real.Add("done-closed", "ws-1", s)

	err := e.Attach(context.Background(), "done-closed", newBlockingConn())
	require.Error(t, err, "Attach on a live-but-dead session must surface the Attach error")
	assert.Contains(t, err.Error(), "terminal: attach:")
}

// TestAttach_PostRestoreVanished covers the post-restore re-fetch guards: after restore
// reports success, the re-fetched session is either gone (!ok) or not live.
func TestAttach_PostRestoreVanished(t *testing.T) {
	dir := t.TempDir()
	ph := session.NewPlaceholder("vanish", "/bin/sh", dir, "", nil)
	live, err := session.New("vanish-live", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	defer live.Kill()

	run := func(t *testing.T, postRestore func() (*session.Session, bool)) error {
		e, real := newCoverEngine(t)
		defer e.Shutdown()
		var cnt int32
		e.reg = &hookedReg{
			Registry: real,
			getFn: func(id string, base *registry.Registry) (*session.Session, bool) {
				if id != "vanish" {
					return base.Get(id)
				}
				switch atomic.AddInt32(&cnt, 1) {
				case 1:
					return ph, true // Attach: not live → restore
				case 2:
					return live, true // restore's internal check: live → restore returns nil (no spawn)
				default:
					return postRestore() // Attach's post-restore re-fetch
				}
			},
		}
		return e.Attach(context.Background(), "vanish", newBlockingConn())
	}

	t.Run("gone", func(t *testing.T) {
		err := run(t, func() (*session.Session, bool) { return nil, false })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "post-restore")
	})

	t.Run("not-live", func(t *testing.T) {
		err := run(t, func() (*session.Session, bool) { return ph, true })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "restore failed")
	})
}

// ---------------------------------------------------------------------------
// Lookup-race (!ok / !wsOK) guards via the hooked registry
// ---------------------------------------------------------------------------

// TestWorkspaceVanished covers the four "WorkspaceID returns false after Get succeeded"
// guards: restore, suspend, persistOnDetach, and flushSessionOnce.
func TestWorkspaceVanished(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	noWs := func(string, *registry.Registry) (string, bool) { return "", false }

	t.Run("restore", func(t *testing.T) {
		e, real := newCoverEngine(t)
		defer e.Shutdown()
		ph := session.NewPlaceholder("ph-ws", "/bin/sh", dir, "", nil)
		real.Add("ph-ws", "ws-1", ph)
		e.reg = &hookedReg{Registry: real, wsFn: noWs}
		err := e.restore(ctx, "ph-ws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace not found")
	})

	t.Run("suspend", func(t *testing.T) {
		e, real := newCoverEngine(t)
		defer e.Shutdown()
		s, err := session.New("live-ws", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
		require.NoError(t, err)
		defer s.Kill()
		real.Add("live-ws", "ws-1", s)
		e.reg = &hookedReg{Registry: real, wsFn: noWs}
		assert.NoError(t, e.suspend(ctx, "live-ws", true),
			"suspend must no-op when the workspace lookup races to empty")
	})

	t.Run("persistOnDetach", func(t *testing.T) {
		e, real := newCoverEngine(t)
		defer e.Shutdown()
		s, err := session.New("live-pd", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
		require.NoError(t, err)
		defer s.Kill()
		real.Add("live-pd", "ws-1", s)
		e.reg = &hookedReg{Registry: real, wsFn: noWs}
		require.NotPanics(t, func() { e.persistOnDetach(ctx, "live-pd", s) })
	})

	t.Run("flushSessionOnce", func(t *testing.T) {
		e, real := newCoverEngine(t)
		defer e.Shutdown()
		s, err := session.New("live-fl", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
		require.NoError(t, err)
		defer s.Kill()
		real.Add("live-fl", "ws-1", s)
		e.reg = &hookedReg{Registry: real, wsFn: noWs}
		require.NotPanics(t, func() { e.flushSessionOnce(ctx, "live-fl") })
	})
}

// TestGhostSessionSkipped covers the "List yields an id that Get cannot resolve" guards in
// Stats, allWorkspaceIDs, placeholderCandidates, and the maintenance underCeiling sweep.
func TestGhostSessionSkipped(t *testing.T) {
	ctx := context.Background()
	e, real := newCoverEngine(t)
	defer e.Shutdown()

	// One real live session so the surrounding loop bodies still run.
	sid, err := e.Create(ctx, "ws-ghost", t.TempDir(), nil)
	require.NoError(t, err)
	defer func() { _ = e.Kill(ctx, sid) }()

	e.reg = &hookedReg{
		Registry: real,
		listFn: func(base *registry.Registry) []string {
			return append(base.List(), "ghost-not-in-map")
		},
	}

	// Stats: the ghost id must be skipped, the real session still counted.
	active, detached, suspended, _, _, _ := e.Stats()
	assert.Equal(t, 1, active+detached+suspended, "ghost id must not be counted")

	assert.NotContains(t, e.allWorkspaceIDs(), "", "ghost id (no workspace) must be skipped")
	assert.Empty(t, e.placeholderCandidates(), "ghost id must not appear as a placeholder candidate")

	// underCeiling sweep iterates the ghost id too; must not panic.
	require.NotPanics(t, func() { e.runMaintenanceOnce(ctx) })
}

// ---------------------------------------------------------------------------
// Stats degraded aggregation (model seam)
// ---------------------------------------------------------------------------

// TestStats_CountsDegraded covers the degraded-session tally: a session whose model reports a
// degraded parse-health surface increments the degraded count and contributes its parse panics.
func TestStats_CountsDegraded(t *testing.T) {
	restore := session.SetNewModelForTest(func(c, r, _ int) (model.TerminalModel, model.Serializer) {
		return &degradedModel{cols: c, rows: r}, degradedSerializer{}
	})
	defer restore()

	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	ctx := context.Background()

	sid, err := e.Create(ctx, "ws-deg", t.TempDir(), nil)
	require.NoError(t, err)
	defer func() { _ = e.Kill(ctx, sid) }()

	_, _, _, _, degraded, parsePanics := e.Stats()
	assert.GreaterOrEqual(t, degraded, 1, "a degraded model must be counted in Stats")
	assert.GreaterOrEqual(t, parsePanics, 7, "the degraded model's parse panics must aggregate")
}

// ---------------------------------------------------------------------------
// Persistence-failure logging branches (deterministic FS errors)
// ---------------------------------------------------------------------------

// TestReapOnDone_DeleteBufError covers the reap-path DeleteBuf error log: the .buf path is an
// undeletable non-empty directory, yet reap still completes and fires ended.
func TestReapOnDone_DeleteBufError(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	ended := make(chan struct{}, 1)
	e.OnSessionEnded(func(context.Context, string, string, int) { ended <- struct{}{} })

	sid, err := e.Create(ctx, "ws-reap", t.TempDir(), nil)
	require.NoError(t, err)
	makeBadBufDir(t, store.dir, sid) // DeleteBuf will fail on reap

	require.NoError(t, e.Kill(ctx, sid))
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("reapOnDone did not complete (ended never fired) past the DeleteBuf error")
	}
}

// TestKillPlaceholder_DeleteBufError covers the placeholder Kill DeleteBuf error log.
func TestKillPlaceholder_DeleteBufError(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	require.NoError(t, e.LoadPlaceholder(ctx, SessionMeta{
		SessionID: "ph-kill", WorkspaceID: "ws-1", Shell: "/bin/sh", State: "suspended",
	}, []byte("CRWB1 80 24 0 10000\n")))
	makeBadBufDir(t, store.dir, "ph-kill")

	require.NoError(t, e.Kill(ctx, "ph-kill"), "Kill must succeed despite the DeleteBuf error")
	assert.False(t, e.SessionExists(ctx, "ph-kill"))
}

// TestDropUnrestorable_DeleteBufError covers the restore-drop DeleteBuf error log: a doomed
// restore (bad shell) drops the placeholder even when its .buf cannot be deleted.
func TestDropUnrestorable_DeleteBufError(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	require.NoError(t, e.LoadPlaceholder(ctx, SessionMeta{
		SessionID: "ph-drop", WorkspaceID: "ws-1", Shell: "/nonexistent/shell/binary", State: "suspended",
	}, nil))
	makeBadBufDir(t, store.dir, "ph-drop")

	err := e.restore(ctx, "ph-drop")
	require.Error(t, err, "restore with a bad shell must fail")
	assert.False(t, e.SessionExists(ctx, "ph-drop"), "the doomed placeholder must still be dropped")
}

// TestEvictPlaceholder_DeleteBufError covers the eviction DeleteBuf error log and the
// not-found early return.
func TestEvictPlaceholder_DeleteBufError(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	restoreN := SetMaxTotalSessionsForTest(1)
	defer restoreN()

	for _, id := range []string{"ev-old", "ev-new"} {
		require.NoError(t, e.LoadPlaceholder(ctx, SessionMeta{
			SessionID: id, WorkspaceID: "ws-1", Shell: "/bin/sh", State: "suspended",
		}, []byte("CRWB1 80 24 0 10000\n")))
	}
	makeBadBufDir(t, store.dir, "ev-old")
	SetLastActiveForTest(e, "ev-old", time.Now().Add(-time.Hour))
	SetLastActiveForTest(e, "ev-new", time.Now())

	e.runMaintenanceOnce(ctx)
	assert.False(t, e.SessionExists(ctx, "ev-old"), "oldest placeholder must be evicted despite the DeleteBuf error")

	// not-found early return.
	require.NotPanics(t, func() { e.evictPlaceholder(ctx, "never-existed") })
}

// TestWriteBufErrors covers the WriteBuf failure logs in suspend, persistOnDetach,
// flushSessionOnce, and Shutdown by pointing the storage dir at a regular file.
func TestWriteBufErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("suspend", func(t *testing.T) {
		e, _ := newCoverEngine(t)
		defer e.Shutdown()
		store := &coverStore{dir: makeFilePath(t)}
		e.SetMetaStore(store)
		sid, err := e.Create(ctx, "ws-1", t.TempDir(), nil)
		require.NoError(t, err)
		defer func() { _ = e.Kill(ctx, sid) }()
		waitEngineIdle(t, e, sid)
		// Force-suspend so it begins regardless of idle timing; WriteBuf will fail.
		require.NoError(t, e.suspend(ctx, sid, true))
		assert.True(t, store.savedState(sid, "suspended"),
			"suspend must persist suspended meta even though the .buf write failed")
	})

	t.Run("persistOnDetach", func(t *testing.T) {
		e, _ := newCoverEngine(t)
		defer e.Shutdown()
		store := &coverStore{dir: makeFilePath(t)}
		e.SetMetaStore(store)
		s, err := session.New("pd-1", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
		require.NoError(t, err)
		defer s.Kill()
		e.reg.Add("pd-1", "ws-1", s)
		require.NotPanics(t, func() { e.persistOnDetach(ctx, "pd-1", s) })
		assert.True(t, store.savedState("pd-1", "detached"))
	})

	t.Run("flushSessionOnce", func(t *testing.T) {
		e, _ := newCoverEngine(t)
		defer e.Shutdown()
		store := &coverStore{dir: makeFilePath(t)}
		e.SetMetaStore(store)
		sid, err := e.Create(ctx, "ws-1", t.TempDir(), nil)
		require.NoError(t, err)
		defer func() { _ = e.Kill(ctx, sid) }()
		waitEngineOutput(t, e, sid) // dirty so Snapshot reports changed and WriteBuf is attempted
		require.NotPanics(t, func() { e.flushSessionOnce(ctx, sid) })
	})

	t.Run("shutdown", func(t *testing.T) {
		e, _ := newCoverEngine(t)
		store := &coverStore{dir: makeFilePath(t)}
		e.SetMetaStore(store)
		sid, err := e.Create(ctx, "ws-1", t.TempDir(), nil)
		require.NoError(t, err)
		_ = sid
		require.NotPanics(t, func() { e.Shutdown() })
	})
}

// TestFlushSessionOnce_NoStore covers the dir=="" early return: with no meta store wired the
// flush resolves an empty dir and returns without writing.
func TestFlushSessionOnce_NoStore(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	sid, err := e.Create(ctx, "ws-1", t.TempDir(), nil)
	require.NoError(t, err)
	defer func() { _ = e.Kill(ctx, sid) }()
	require.NotPanics(t, func() { e.flushSessionOnce(ctx, sid) })
}

// ---------------------------------------------------------------------------
// Maintenance phase-3 ceiling branches
// ---------------------------------------------------------------------------

// TestMaintenance_AttachedSessionSkipped covers the AttachedCount>0 skips in both the
// per-workspace and global classification loops: an attached session is never a suspend
// candidate.
func TestMaintenance_AttachedSessionSkipped(t *testing.T) {
	restore := SetSoftLimitPerWorkspaceForTest(0) // force the soft-limit loop body to run
	defer restore()

	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	sid, err := e.Create(ctx, "ws-att", t.TempDir(), nil)
	require.NoError(t, err)

	attachReturned := make(chan struct{})
	conn := newBlockingConn()
	go func() {
		_ = e.Attach(ctx, sid, conn)
		close(attachReturned)
	}()
	require.Eventually(t, func() bool {
		s, ok := e.StateOf(sid)
		return ok && s == "active"
	}, 5*time.Second, 20*time.Millisecond, "session must become active (attached)")

	// Stats must also classify the attached session as active (the "active" tally branch).
	active, _, _, _, _, _ := e.Stats()
	assert.GreaterOrEqual(t, active, 1, "an attached session must be counted as active by Stats")

	e.runMaintenanceOnce(ctx)
	st, _ := e.StateOf(sid)
	assert.Equal(t, "active", st, "an attached session must never be suspended by maintenance")

	conn.Close()
	<-attachReturned
	_ = e.Kill(ctx, sid)
}

// TestMaintenance_Phase3aIdleSuspend covers the global-ceiling idle-suspend path (phase 3a):
// two idle detached sessions over the byte ceiling, where dropping caches is insufficient but
// suspending the oldest idle session brings the engine back under.
func TestMaintenance_Phase3aIdleSuspend(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	sid1, err := e.Create(ctx, "ws-3a", t.TempDir(), nil)
	require.NoError(t, err)
	sid2, err := e.Create(ctx, "ws-3a", t.TempDir(), nil)
	require.NoError(t, err)
	waitEngineIdle(t, e, sid1)
	waitEngineIdle(t, e, sid2)

	// Fresh model bytes (no caches yet) ≈ two grids. A ceiling at 3/4 of that means cache
	// drops cannot get under (two grids remain) but one idle suspend (grid → tiny placeholder)
	// does.
	_, _, _, fresh, _, _ := e.Stats()
	restoreB := SetMaxTotalModelBytesForTest(fresh * 3 / 4)
	defer restoreB()

	base := time.Now().Add(-time.Hour)
	SetLastActiveForTest(e, sid1, base)
	SetLastActiveForTest(e, sid2, base.Add(time.Minute))

	// Suspend runs synchronously inside the sweep, but the idle check (TIOCGPGRP)
	// can transiently read non-idle under load and skip that pass; production retries
	// via the maintenance ticker. Drive maintenance until the suspend lands — the loop
	// exit IS the assertion, and a genuine "never suspends" bug is caught by the
	// go test -timeout backstop rather than a hard-coded per-test deadline.
	for !store.savedState(sid1, "suspended") {
		e.runMaintenanceOnce(ctx)
	}
	assert.NoError(t, e.Write(ctx, sid2, []byte("echo alive\n")), "the newer session must stay live")
	_ = e.Kill(ctx, sid1)
	_ = e.Kill(ctx, sid2)
}

// TestMaintenance_Phase3aSuspendLastThenReturn covers the underCeiling return AFTER the last
// idle suspend (the guard just before placeholder eviction): a single idle session whose
// suspension brings the engine back under.
func TestMaintenance_Phase3aSuspendLastThenReturn(t *testing.T) {
	ctx := context.Background()
	e, _ := newCoverEngine(t)
	defer e.Shutdown()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)

	sid, err := e.Create(ctx, "ws-3a1", t.TempDir(), nil)
	require.NoError(t, err)
	waitEngineIdle(t, e, sid)

	_, _, _, fresh, _, _ := e.Stats()
	restoreB := SetMaxTotalModelBytesForTest(fresh / 2) // suspending the only session goes under
	defer restoreB()
	SetLastActiveForTest(e, sid, time.Now().Add(-time.Hour))

	// Suspend runs synchronously inside the sweep, but the idle check (TIOCGPGRP)
	// can transiently read non-idle under load and skip that pass; production retries
	// via the ticker. Drive maintenance until the suspend lands — the loop exit IS the
	// assertion; a genuine failure is caught by the go test -timeout backstop.
	for !store.savedState(sid, "suspended") {
		e.runMaintenanceOnce(ctx)
	}
	_ = e.Kill(ctx, sid)
}

// TestMaintenance_CacheDropResolves covers the phase-3 cache-reclaim returns: dropping the
// reclaimable cached blob(s) alone brings the engine under the byte ceiling, so no session is
// suspended. Exercised with two sessions (mid-loop return) and one session (post-loop return).
func TestMaintenance_CacheDropResolves(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T, n int) {
		e, _ := newCoverEngine(t)
		defer e.Shutdown()
		store := &coverStore{dir: t.TempDir()}
		e.SetMetaStore(store)

		var sids []string
		for i := 0; i < n; i++ {
			sid, err := e.Create(ctx, "ws-cache", t.TempDir(), nil)
			require.NoError(t, err)
			sids = append(sids, sid)
			waitEngineIdle(t, e, sid)
		}
		// Prime caches: a maintenance pass under a huge ceiling Snapshots each session
		// (populating lastBlob) without suspending anything.
		e.runMaintenanceOnce(ctx)

		// Now set the ceiling 1 byte under the cached total. Dropping any cached blob (each is
		// far larger than 1 byte) immediately brings us back under, so the cache-reclaim
		// pre-step resolves the pressure and nothing is suspended.
		_, _, _, total, _, _ := e.Stats()
		require.Greater(t, total, int64(1))
		restoreB := SetMaxTotalModelBytesForTest(total - 1)

		e.runMaintenanceOnce(ctx)
		restoreB()

		for _, sid := range sids {
			assert.NoError(t, e.Write(ctx, sid, []byte("echo still\n")),
				"cache reclaim alone must resolve the ceiling; no session may be suspended")
			_ = e.Kill(ctx, sid)
		}
	}

	t.Run("two-sessions-midloop", func(t *testing.T) { run(t, 2) })
	t.Run("one-session-postloop", func(t *testing.T) { run(t, 1) })
}

// ---------------------------------------------------------------------------
// Maintenance ticker branch
// ---------------------------------------------------------------------------

// TestMaintenanceLoop_TickerFires covers the maintenanceLoop ticker case: with a short
// interval the background sweep runs on its own and flushes a dirty session's .buf to disk.
func TestMaintenanceLoop_TickerFires(t *testing.T) {
	origInterval := maintenanceTickInterval
	maintenanceTickInterval = 5 * time.Millisecond
	e := New().(*terminalEngine)
	defer func() {
		e.Shutdown()
		maintenanceTickInterval = origInterval
	}()

	ctx := context.Background()
	store := &coverStore{dir: t.TempDir()}
	e.SetMetaStore(store)
	sid, err := e.Create(ctx, "ws-tick", t.TempDir(), nil)
	require.NoError(t, err)

	// The ticker-driven sweep must flush the dirty session's scrollback to disk on its own.
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(filepath.Join(store.dir, sid+".buf"))
		return statErr == nil
	}, 5*time.Second, 20*time.Millisecond, "the maintenance ticker must drive a cadence flush")
	_ = e.Kill(ctx, sid)
}

// ---------------------------------------------------------------------------
// local wait helpers (internal package)
// ---------------------------------------------------------------------------

func waitEngineIdle(t *testing.T, e *terminalEngine, sid string) {
	t.Helper()
	require.Eventually(t, func() bool {
		s, ok := e.reg.Get(sid)
		return ok && s.IsLive() && s.IsIdle()
	}, 15*time.Second, 50*time.Millisecond, "session %s did not become idle", sid)
	// Extra settle so the prompt output has been fully consumed by the pump: wait
	// until the serialized model stops growing (N consecutive equal-length samples)
	// instead of a blind sleep.
	s, ok := e.reg.Get(sid)
	require.True(t, ok, "session %s vanished before settle", sid)
	lastLen, stable := -1, 0
	require.Eventually(t, func() bool {
		cur := s.SerializedLen()
		if cur == lastLen {
			stable++
		} else {
			stable, lastLen = 0, cur
		}
		return stable >= 3
	}, 10*time.Second, 30*time.Millisecond, "session %s output did not settle", sid)
}

func waitEngineOutput(t *testing.T, e *terminalEngine, sid string) {
	t.Helper()
	require.Eventually(t, func() bool {
		s, ok := e.reg.Get(sid)
		return ok && s.SerializedLen() > 0
	}, 10*time.Second, 20*time.Millisecond, "session %s produced no output", sid)
}
