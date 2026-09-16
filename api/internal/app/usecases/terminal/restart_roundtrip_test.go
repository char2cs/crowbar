package terminal_test

// TestRegression_TerminalSession_RestartRoundTrip_RealStore is the
// cross-boundary, engine-level end-to-end test for terminal session persistence.
// It uses the REAL GORM SQLite session store and real disk-backed scrollback
// persistence — NOT a fake — and crosses the fresh-engine + real-disk-store
// boundary to prove the headline guarantee:
//
//   Sessions survive a simulated daemon restart; scrollback (including known
//   output driven into the session before shutdown) is replayed on the next
//   client attach, and the session restores in the saved CWD.
//
// Fidelity: engine + real GORM SQLite store (no HTTP/WS layer). See
// api/tests/integration/terminal/ for the full REST/WS-level variant.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ---------------------------------------------------------------------------
// restartConn — minimal WSConn that captures received frames.
// ---------------------------------------------------------------------------

// restartFrame is the decoded wire frame the engine's writePump emits. Snapshot
// marks a serialized SCREEN-MODEL redraw (the scrollback replay handed to a
// freshly attached client); a frame without it is live output the restored PTY
// produced just now.
type restartFrame struct {
	Data     string `json:"data"`
	Snapshot bool   `json:"snapshot"`
}

// restartConn implements core/terminal.WSConn. WriteMessage decodes each frame
// into an in-memory slice (never errors) and broadcasts on a sync.Cond so a
// waiter wakes on the REAL arrival of output; ReadMessage blocks until the conn
// is closed. This lets us capture all PTY output for assertion without blocking
// the session on a network — and without a single clock.
type restartConn struct {
	mu        sync.Mutex
	cond      *sync.Cond
	inbox     []restartFrame
	closed    chan struct{}
	closeOnce sync.Once
}

func newRestartConn() *restartConn {
	c := &restartConn{closed: make(chan struct{})}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (r *restartConn) WriteMessage(_ int, data []byte) error {
	var f restartFrame
	_ = json.Unmarshal(data, &f) // a frame we cannot decode simply matches nothing
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inbox = append(r.inbox, f)
	r.cond.Broadcast()
	return nil
}

func (r *restartConn) ReadMessage() (int, []byte, error) {
	<-r.closed
	return 0, nil, &restartConnClosedErr{}
}

func (r *restartConn) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cond.Broadcast() // wake any waiter so it re-evaluates against the final inbox
	return nil
}

// waitFor blocks until some received frame satisfies pred, woken by this conn's
// own WriteMessage — a REAL signal, never a poll.
//
// There is deliberately NO deadline: a PTY-backed login shell's start-up cost is
// the user's ~/.zshrc, which under CPU load runs an order of magnitude slower
// than on an idle machine. Any fixed wait here is a GUESS about someone else's
// shell, and the previous 5 s guess is exactly what made this test fail in the
// full suite while passing alone. Blocking on the arrival of output instead makes
// a slow machine merely slow, never red; `go test -timeout` is the only backstop.
func (r *restartConn) waitFor(pred func(restartFrame) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		for _, f := range r.inbox {
			if pred(f) {
				return
			}
		}
		r.cond.Wait()
	}
}

// waitForData blocks until any received frame's payload satisfies pred.
func (r *restartConn) waitForData(pred func(string) bool) {
	r.waitFor(func(f restartFrame) bool { return pred(f.Data) })
}

type restartConnClosedErr struct{}

func (e *restartConnClosedErr) Error() string { return "restartConn: connection closed" }

// rrHas reports whether s contains sub (simple substring check).
func rrHas(s, sub string) bool {
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
// TestRegression_TerminalSession_RestartRoundTrip_RealStore
// ---------------------------------------------------------------------------

// TestRegression_TerminalSession_RestartRoundTrip_RealStore proves the full
// restart round-trip over the REAL disk-backed GORM store.
//
// Steps:
//  1. Build a real GORM SQLite session store over a temp file (mirrors
//     production wiring of TerminalSession store + crowbarHome).
//  2. Engine #1 (real metaStore injected): create session, drive known output
//     ("restart-crossboundary-marker") into the PTY ring, capture the CWD,
//     then call Shutdown() (graceful flush + persist with state="suspended").
//  3. Close the DB connection (simulates daemon exit releasing the file lock).
//  4. Engine #2 (fresh engine, same real DB file): call RestorePersistedSessions;
//     assert the session appears as a suspended placeholder via SessionExists /
//     StateOf / ListSessionsForChat.
//  5. Attach a new capture conn to engine #2; assert the replayed scrollback
//     contains the pre-restart marker AND the session restores live in the
//     saved CWD.
func TestRegression_TerminalSession_RestartRoundTrip_RealStore(t *testing.T) {
	ctx := context.Background()

	// On-disk state shared across both engine phases.
	home := t.TempDir() // crowbar home root (mirrors ~/.crowbar in production)
	cwd := t.TempDir()  // session working directory — must exist on disk

	const (
		projectID   = "proj-rrt"
		repoID      = "repo-rrt"
		workspaceID = "ws-rrt"
		chatID      = "chat-rrt"
	)

	// A minimal WorktreeResolver mapping the owning CHAT to the workspace behind
	// its worktree. Both engine phases share this same in-memory resolver (the
	// chat→workspace mapping is stable across the restart).
	worktrees := &fakeWorktreeResolver{
		ws: domain.Workspace{
			ID:           workspaceID,
			ProjectID:    projectID,
			RepoID:       repoID,
			WorktreePath: cwd,
		},
	}

	// ---- Real GORM SQLite session store (shared on-disk DB file). ----
	//
	// Using a real file (not :memory:) so engine #2 reads what engine #1 wrote.
	dbPath := filepath.Join(home, "view.db")
	db1, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err, "open real SQLite DB for engine #1")
	sessStore1, err := storesqlite.NewFromDB[domain.TerminalSession, string](db1)
	require.NoError(t, err, "create TerminalSession store for engine #1")

	// Real SessionMetaStore that delegates to the GORM store and resolves
	// StorageDir via crowbarHome + workspace ProjectID/RepoID (leaf: the chat).
	ms1 := terminal.NewSessionMetaStore(
		worktrees,
		sessStore1,
		func() (string, error) { return home, nil },
	)

	// ---- Engine #1: create, drive output, graceful Shutdown. ----
	//
	// The background 10-second maintenance ticker is not stopped here because
	// StopMaintenanceForTest is package-internal to the engine. It cannot
	// interfere with the assertions regardless of how long a loaded machine
	// makes this test take: a session with a client attached is ineligible for
	// suspension, the sweep's only other effect is a scrollback flush (which is
	// exactly what this test wants persisted anyway), and all shared state is
	// mutex-protected (race-safe). Nothing below depends on the test finishing
	// within a tick.
	eng1 := engineterminal.New()
	eng1.SetMetaStore(ms1)

	profiles1 := mocks.NewTerminalProfileStore()
	uc1 := terminal.New(eng1, profiles1, worktrees, ms1)

	sessionID, err := uc1.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "engine #1: CreateSession must succeed")

	// Attach a capture conn to receive PTY frames and verify output.
	conn1 := newRestartConn()
	attachDone1 := make(chan struct{})
	go func() {
		defer close(attachDone1)
		_ = eng1.Attach(ctx, sessionID, conn1)
	}()

	// Block until the shell emits its initial prompt — the real signal that the
	// PTY is live and the model has content Shutdown can flush to disk.
	conn1.waitForData(func(data string) bool { return len(data) > 0 })

	// Inject a known marker so we can assert scrollback content post-restart, and
	// block until the PTY echoes it back — the real signal that the marker is in
	// the model (and so will be in the .buf Shutdown writes).
	require.NoError(t, eng1.Write(ctx, sessionID, []byte("echo restart-crossboundary-marker\n")))
	conn1.waitForData(func(data string) bool { return rrHas(data, "restart-crossboundary-marker") })

	// Capture the CWD the session was created in so we can assert it post-restart.
	savedCWD := cwd

	// Detach the capture conn — triggers persistOnDetach (flushes ring to disk
	// as state="detached"). This is serialised: <-attachDone1 guarantees
	// persistOnDetach has completed before we proceed.
	conn1.Close()
	<-attachDone1

	// Graceful Shutdown: for every live session, Shutdown
	//   a) flushes the ring snapshot to disk (WriteBuf)
	//   b) saves meta with state="suspended" in the REAL SQLite DB
	//   c) marks suspending so reapOnDone skips the delete-on-reap path
	//   d) removes from registry and kills the PTY process
	// After this call, the session row in the DB has state="suspended" and the
	// .buf file contains the full scrollback including our marker.
	eng1.Shutdown()

	// Release the SQLite connection so engine #2 can open the same DB file.
	// (SQLite WAL allows concurrent readers but we close explicitly for clarity.)
	{
		sqlDB, dbErr := db1.DB()
		require.NoError(t, dbErr, "get sql.DB handle to close engine #1 connection")
		require.NoError(t, sqlDB.Close(), "close engine #1 SQLite connection")
	}

	// ---- Engine #2: fresh engine + same real DB → RestorePersistedSessions. ----
	db2, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err, "open real SQLite DB for engine #2")
	sessStore2, err := storesqlite.NewFromDB[domain.TerminalSession, string](db2)
	require.NoError(t, err, "create TerminalSession store for engine #2")
	t.Cleanup(func() {
		sqlDB, _ := db2.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	ms2 := terminal.NewSessionMetaStore(
		worktrees,
		sessStore2,
		func() (string, error) { return home, nil },
	)

	eng2 := engineterminal.New()
	eng2.SetMetaStore(ms2)

	profiles2 := mocks.NewTerminalProfileStore()
	uc2 := terminal.New(eng2, profiles2, worktrees, ms2)

	// RestorePersistedSessions reads the REAL SQLite DB, finds the session row
	// written by engine #1, and loads it as a PTY-less placeholder in engine #2.
	require.NoError(t, uc2.RestorePersistedSessions(ctx),
		"engine #2: RestorePersistedSessions must succeed")

	// ---- Assert: session is registered as a suspended placeholder. ----
	assert.True(t, eng2.SessionExists(ctx, sessionID),
		"engine #2: session must be in registry after RestorePersistedSessions")

	state, ok := eng2.StateOf(sessionID)
	assert.True(t, ok, "engine #2: StateOf must return ok=true for restored session")
	assert.Equal(t, "suspended", state,
		"engine #2: restored session must be a suspended placeholder")

	assert.Contains(t, eng2.ListSessionsForChat(chatID), sessionID,
		"engine #2: ListSessionsForChat must include the restored session")

	// ---- Attach to the restored session — verify scrollback replay + CWD. ----
	//
	// Attach transparently restores the placeholder: a fresh PTY shell is
	// spawned with the saved CWD, and the stored scrollback is replayed first.
	conn2 := newRestartConn()
	attachDone2 := make(chan struct{})
	go func() {
		defer close(attachDone2)
		_ = eng2.Attach(ctx, sessionID, conn2)
	}()

	// The replayed scrollback must contain the pre-restart marker. This lands in
	// the SNAPSHOT frame the writePump hands a freshly attached client, and it is
	// read from disk — so it arrives immediately and says nothing about whether
	// the re-spawned shell came up.
	conn2.waitForData(func(data string) bool { return rrHas(data, "restart-crossboundary-marker") })

	// Prove the restore produced a WORKING shell by DRIVING output, not by
	// waiting for a spontaneous one. A freshly re-spawned login shell may fold
	// its only spontaneous output — the prompt/title — into the scrollback-replay
	// SNAPSHOT frame above; when it does, no non-snapshot frame ever follows, and
	// a wait for spontaneous live output blocks until go test's whole-binary
	// timeout, taking every test in this package's binary down with it. (That is
	// the rare full-suite CI hang the earlier 5 s-deadline version papered over —
	// removing the deadline turned a fast flake into a 10-minute one.) Echo a
	// marker that cannot exist in the pre-restart scrollback and block on its
	// echo instead: the shell must read input, run the command, and emit for that
	// marker to arrive, which is a complete and deterministic proof of a live
	// restored shell — and, unlike a prompt, cannot be pre-folded into a snapshot.
	const liveMarker = "restored-shell-live-marker"
	require.NoError(t, eng2.Write(ctx, sessionID, []byte("echo "+liveMarker+"\n")))
	conn2.waitForData(func(data string) bool { return rrHas(data, liveMarker) })

	// The restored shell must run in the saved CWD (the working directory from
	// before the restart). We write `pwd` and block until the path comes back.
	require.NoError(t, eng2.Write(ctx, sessionID, []byte("pwd\n")))
	conn2.waitForData(func(data string) bool { return rrHas(data, savedCWD) })

	// Also verify the scrollback file still exists and the meta row is readable.
	storageDir, storErr := ms2.StorageDir(ctx, chatID)
	require.NoError(t, storErr, "engine #2: must resolve storage dir for the owning chat")
	bufPath := filepath.Join(storageDir, sessionID+".buf")
	_, statErr := os.Stat(bufPath)
	assert.NoError(t, statErr, "scrollback .buf file must exist after restart-restore")

	// ---- Cleanup: kill the session and stop engine #2. ----
	conn2.Close()
	<-attachDone2
	_ = eng2.Kill(ctx, sessionID)
	eng2.Shutdown()
}

// ---------------------------------------------------------------------------
// TestRegression_TerminalSession_MetaRowSurvivesShutdown_RealStore
// ---------------------------------------------------------------------------

// TestRegression_TerminalSession_MetaRowSurvivesShutdown_RealStore is a focused
// sub-guarantee test: after a graceful Shutdown, the REAL SQLite session row
// has state="suspended" and the scrollback .buf file exists on disk, without
// needing to spin up engine #2. This regression locks the persistence contract
// independently of the full restart round-trip.
func TestRegression_TerminalSession_MetaRowSurvivesShutdown_RealStore(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	cwd := t.TempDir()

	const (
		projectID   = "proj-mrs"
		repoID      = "repo-mrs"
		workspaceID = "ws-mrs"
		chatID      = "chat-mrs"
	)

	worktrees := &fakeWorktreeResolver{
		ws: domain.Workspace{
			ID:           workspaceID,
			ProjectID:    projectID,
			RepoID:       repoID,
			WorktreePath: cwd,
		},
	}

	dbPath := filepath.Join(home, "view.db")
	db, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	sessStore, err := storesqlite.NewFromDB[domain.TerminalSession, string](db)
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	ms := terminal.NewSessionMetaStore(
		worktrees,
		sessStore,
		func() (string, error) { return home, nil },
	)

	eng := engineterminal.New()
	eng.SetMetaStore(ms)

	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, worktrees, ms)

	sessionID, err := uc.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "CreateSession must succeed")

	// Attach a capture conn and wait for the initial shell prompt so the ring
	// buffer has content for Shutdown to flush (empty ring → empty .buf).
	conn := newRestartConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sessionID, conn)
	}()

	// Block on the shell's first byte — the real signal that the model has
	// content for Shutdown to flush.
	conn.waitForData(func(data string) bool { return len(data) > 0 })

	// Graceful shutdown persists scrollback + state="suspended".
	// The conn is still attached; Shutdown handles that path.
	eng.Shutdown()
	<-attachDone // wait for Attach to return after the session is killed

	// ---- Directly read from the REAL SQLite store — no engine #2 needed. ----
	row, findErr := sessStore.FindByKey(ctx, sessionID)
	require.NoError(t, findErr, "FindByKey must succeed on real SQLite store")
	require.NotNil(t, row, "session row must exist in real SQLite store after Shutdown")
	assert.Equal(t, "suspended", row.State,
		"row state must be 'suspended' after Shutdown")
	assert.Equal(t, chatID, row.ChatID,
		"row must carry the correct chatID")
	assert.Equal(t, projectID, row.ProjectID,
		"row must carry the correct projectID")

	// Scrollback .buf must be present on disk.
	storageDir, storErr := ms.StorageDir(ctx, chatID)
	require.NoError(t, storErr, "StorageDir must resolve")
	bufPath := filepath.Join(storageDir, sessionID+".buf")
	_, statErr := os.Stat(bufPath)
	assert.NoError(t, statErr, "scrollback .buf must exist after Shutdown")
}

// ---------------------------------------------------------------------------
// TestRegression_CommandSession_NotPersistedAcrossRestart
// ---------------------------------------------------------------------------

// TestRegression_CommandSession_NotPersistedAcrossRestart is the terminal half
// of the "dead agent PTY reads as alive after a daemon restart" bug.
//
// A COMMAND session (engine.CreateCommand — an agentic vendor CLI spawned with
// an explicit argv) is unrestorable: the restore path only knows how to birth a
// LOGIN SHELL, so bringing one back would exec the joined argv string as a bogus
// binary — or, worse, hand the agent's chat pane a BARE SHELL under the dead
// CLI's session id. Before the fix, graceful Shutdown persisted command sessions
// like any other, and the next boot's RestorePersistedSessions dutifully reloaded
// one as a PTY-less placeholder — which the agent boot reconcile then read as
// "the CLI survived", leaving the chat's segment active forever.
//
// This pins BOTH halves of the repair over the real disk-backed store:
//
//   - a command session leaves NO durable row and is NOT restored on the next
//     boot (it is genuinely absent from the fresh engine's registry), while
//   - an ordinary SHELL session still round-trips exactly as before (no
//     regression to terminal-tab persistence) — and, restored, it is a
//     placeholder: SessionExists (registered) is true but SessionLive (backed by
//     a live PTY) is false, the distinction the agent reconcile now relies on.
//
// No timing: every step blocks on a real signal (CreateCommand returns once the
// PTY is started; Shutdown is synchronous).
func TestRegression_CommandSession_NotPersistedAcrossRestart(t *testing.T) {
	ctx := context.Background()

	home := t.TempDir()
	cwd := t.TempDir()

	const (
		projectID   = "proj-cmd"
		repoID      = "repo-cmd"
		workspaceID = "ws-cmd"
		chatID      = "chat-cmd"
	)

	worktrees := &fakeWorktreeResolver{
		ws: domain.Workspace{
			ID:           workspaceID,
			ProjectID:    projectID,
			RepoID:       repoID,
			WorktreePath: cwd,
		},
	}

	dbPath := filepath.Join(home, "view.db")
	db1, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	sessStore1, err := storesqlite.NewFromDB[domain.TerminalSession, string](db1)
	require.NoError(t, err)
	ms1 := terminal.NewSessionMetaStore(worktrees, sessStore1, func() (string, error) { return home, nil })

	eng1 := engineterminal.New()
	eng1.SetMetaStore(ms1)
	uc1 := terminal.New(eng1, mocks.NewTerminalProfileStore(), worktrees, ms1)

	// The control: an ordinary shell terminal tab, which MUST still survive.
	shellID, err := uc1.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "engine #1: shell CreateSession must succeed")

	// The subject: an agentic vendor CLI. `cat` stands in for claude/codex — it
	// stays alive on its PTY, so Shutdown sees it as a LIVE session (the exact
	// state that used to get it persisted).
	cmdID, err := eng1.CreateCommand(ctx, chatID, cwd, []string{"cat"}, nil, nil)
	require.NoError(t, err, "engine #1: CreateCommand must succeed")
	require.True(t, eng1.SessionLive(ctx, cmdID), "precondition: the vendor CLI's PTY is live before shutdown")

	// Graceful daemon stop.
	eng1.Shutdown()

	// The command session must have left NO durable row behind.
	cmdRow, err := sessStore1.FindByKey(ctx, cmdID)
	require.NoError(t, err)
	assert.Nil(t, cmdRow,
		"a command session (agentic vendor CLI) must never be persisted: it is unrestorable")

	shellRow, err := sessStore1.FindByKey(ctx, shellID)
	require.NoError(t, err)
	require.NotNil(t, shellRow, "the ordinary shell session must still be persisted (no regression)")
	assert.Equal(t, "suspended", shellRow.State)

	{
		sqlDB, dbErr := db1.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	}

	// ---- Engine #2: the next daemon boot over the same on-disk state. ----
	db2, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	sessStore2, err := storesqlite.NewFromDB[domain.TerminalSession, string](db2)
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, _ := db2.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	ms2 := terminal.NewSessionMetaStore(worktrees, sessStore2, func() (string, error) { return home, nil })

	eng2 := engineterminal.New()
	eng2.SetMetaStore(ms2)
	uc2 := terminal.New(eng2, mocks.NewTerminalProfileStore(), worktrees, ms2)
	t.Cleanup(eng2.Shutdown)

	require.NoError(t, uc2.RestorePersistedSessions(ctx))

	// The dead vendor CLI is genuinely gone — the agent boot reconcile can now
	// see that its process did not survive and end the chat's segment.
	assert.False(t, eng2.SessionExists(ctx, cmdID),
		"a command session must NOT be restored as a placeholder after a restart")
	assert.False(t, eng2.SessionLive(ctx, cmdID),
		"a command session's PTY is dead after a restart — SessionLive must say so")

	// The shell tab still round-trips — but as a PTY-LESS placeholder: registered
	// (SessionExists) yet not backed by a live process (SessionLive).
	assert.True(t, eng2.SessionExists(ctx, shellID),
		"an ordinary shell session must still be restored (no regression)")
	assert.False(t, eng2.SessionLive(ctx, shellID),
		"a restored session is a placeholder: registered, but its process is dead")
}
