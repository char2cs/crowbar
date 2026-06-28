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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ---------------------------------------------------------------------------
// restartConn — minimal WSConn that captures received frames.
// ---------------------------------------------------------------------------

// restartConn implements engine/terminal.WSConn. WriteMessage appends frames
// to an in-memory slice (never errors), ReadMessage blocks until the conn is
// closed. This lets us capture all PTY output for assertion without blocking
// the session on a network.
type restartConn struct {
	mu        sync.Mutex
	inbox     [][]byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newRestartConn() *restartConn {
	return &restartConn{closed: make(chan struct{})}
}

func (r *restartConn) WriteMessage(_ int, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	r.mu.Lock()
	r.inbox = append(r.inbox, cp)
	r.mu.Unlock()
	return nil
}

func (r *restartConn) ReadMessage() (int, []byte, error) {
	<-r.closed
	return 0, nil, &restartConnClosedErr{}
}

func (r *restartConn) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

// hasContent reports whether any received frame satisfies pred.
func (r *restartConn) hasContent(pred func(string) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, raw := range r.inbox {
		var msg struct {
			Data string `json:"data"`
		}
		if json.Unmarshal(raw, &msg) == nil && pred(msg.Data) {
			return true
		}
	}
	return false
}

type restartConnClosedErr struct{}

func (e *restartConnClosedErr) Error() string { return "restartConn: connection closed" }

// waitForRestartContent polls conn.hasContent until pred returns true or timeout.
func waitForRestartContent(t *testing.T, conn *restartConn, pred func(string) bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if conn.hasContent(pred) {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
}

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
//     StateOf / ListSessionsForWorkspace.
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
	)

	// A minimal WorkspaceRepo that always returns our known workspace. Both
	// engine phases share this same in-memory repo (the workspace state is
	// stable across the restart).
	wsRepo := &fakeWorkspaceRepo{
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
	// StorageDir via crowbarHome + workspace ProjectID/RepoID.
	ms1 := terminal.NewSessionMetaStore(
		wsRepo,
		sessStore1,
		func() (string, error) { return home, nil },
	)

	// ---- Engine #1: create, drive output, graceful Shutdown. ----
	//
	// The background 10-second maintenance ticker is not stopped here because
	// StopMaintenanceForTest is package-internal to the engine. The ticker
	// cannot interfere: (a) the session has a client attached so it's
	// ineligible for suspension, (b) the test drives and shuts down well
	// within 10 s, and (c) all shared state is mutex-protected (race-safe).
	eng1 := engineterminal.New()
	eng1.SetMetaStore(ms1)

	profiles1 := mocks.NewTerminalProfileStore()
	uc1 := terminal.New(eng1, profiles1, wsRepo, ms1)

	sessionID, err := uc1.CreateSession(ctx, workspaceID, nil)
	require.NoError(t, err, "engine #1: CreateSession must succeed")

	// Attach a capture conn to receive PTY frames and verify output.
	conn1 := newRestartConn()
	attachDone1 := make(chan struct{})
	go func() {
		defer close(attachDone1)
		_ = eng1.Attach(ctx, sessionID, conn1)
	}()

	// Wait for the shell to emit its initial prompt — confirms the PTY is live
	// and the ring buffer has content that Shutdown can flush to disk.
	require.True(t,
		waitForRestartContent(t, conn1, func(data string) bool { return len(data) > 0 }, 10*time.Second),
		"engine #1: must receive initial PTY output (shell prompt)")

	// Inject a known marker so we can assert scrollback content post-restart.
	require.NoError(t, eng1.Write(ctx, sessionID, []byte("echo restart-crossboundary-marker\n")))
	require.True(t,
		waitForRestartContent(t, conn1, func(data string) bool {
			return rrHas(data, "restart-crossboundary-marker")
		}, 5*time.Second),
		"engine #1: must receive 'restart-crossboundary-marker' in PTY output")

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
		wsRepo,
		sessStore2,
		func() (string, error) { return home, nil },
	)

	eng2 := engineterminal.New()
	eng2.SetMetaStore(ms2)

	profiles2 := mocks.NewTerminalProfileStore()
	uc2 := terminal.New(eng2, profiles2, wsRepo, ms2)

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

	assert.Contains(t, eng2.ListSessionsForWorkspace(workspaceID), sessionID,
		"engine #2: ListSessionsForWorkspace must include the restored session")

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

	// The replayed scrollback must contain the pre-restart marker.
	assert.True(t,
		waitForRestartContent(t, conn2, func(data string) bool {
			return rrHas(data, "restart-crossboundary-marker")
		}, 10*time.Second),
		"engine #2: scrollback replay must contain the pre-restart marker 'restart-crossboundary-marker'")

	// The restored shell must run in the saved CWD (the working directory from
	// before the restart). We write `pwd` and verify the output.
	require.NoError(t, eng2.Write(ctx, sessionID, []byte("pwd\n")))
	assert.True(t,
		waitForRestartContent(t, conn2, func(data string) bool {
			return rrHas(data, savedCWD)
		}, 5*time.Second),
		"engine #2: restored session must run in saved CWD (%s)", savedCWD)

	// Also verify the scrollback file still exists and the meta row is readable.
	storageDir, storErr := ms2.StorageDir(ctx, workspaceID)
	require.NoError(t, storErr, "engine #2: must resolve storage dir for workspace")
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
	)

	wsRepo := &fakeWorkspaceRepo{
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
		wsRepo,
		sessStore,
		func() (string, error) { return home, nil },
	)

	eng := engineterminal.New()
	eng.SetMetaStore(ms)

	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, wsRepo, ms)

	sessionID, err := uc.CreateSession(ctx, workspaceID, nil)
	require.NoError(t, err, "CreateSession must succeed")

	// Attach a capture conn and wait for the initial shell prompt so the ring
	// buffer has content for Shutdown to flush (empty ring → empty .buf).
	conn := newRestartConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sessionID, conn)
	}()

	require.True(t,
		waitForRestartContent(t, conn, func(data string) bool { return len(data) > 0 }, 10*time.Second),
		"must receive initial PTY output before Shutdown")

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
	assert.Equal(t, workspaceID, row.WorkspaceID,
		"row must carry the correct workspaceID")
	assert.Equal(t, projectID, row.ProjectID,
		"row must carry the correct projectID")

	// Scrollback .buf must be present on disk.
	storageDir, storErr := ms.StorageDir(ctx, workspaceID)
	require.NoError(t, storErr, "StorageDir must resolve")
	bufPath := filepath.Join(storageDir, sessionID+".buf")
	_, statErr := os.Stat(bufPath)
	assert.NoError(t, statErr, "scrollback .buf must exist after Shutdown")
}
