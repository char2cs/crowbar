package terminal_test

import (
	"context"
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
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type restartFrame struct {
	Data     string
	Snapshot bool
}

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
	if len(data) > 0 && data[0] <= engineterminal.FrameSnapshot {
		f = restartFrame{Data: string(data[1:]), Snapshot: data[0] == engineterminal.FrameSnapshot}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inbox = append(r.inbox, f)
	r.cond.Broadcast()
	return nil
}

func (r *restartConn) SetWriteDeadline(time.Time) error { return nil }

func (r *restartConn) ReadMessage() (int, []byte, error) {
	<-r.closed
	return 0, nil, &restartConnClosedErr{}
}

func (r *restartConn) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cond.Broadcast()
	return nil
}

const restartWait = 2 * time.Minute

func (r *restartConn) waitFor(t *testing.T, pred func(restartFrame) bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), restartWait)
	defer cancel()
	defer context.AfterFunc(ctx, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cond.Broadcast()
	})()
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		for _, f := range r.inbox {
			if pred(f) {
				return
			}
		}
		if ctx.Err() != nil {
			t.Fatalf("no frame matched within %s", restartWait)
		}
		r.cond.Wait()
	}
}

func (r *restartConn) waitForData(t *testing.T, pred func(string) bool) {
	t.Helper()
	r.waitFor(t, func(f restartFrame) bool { return pred(f.Data) })
}

type restartConnClosedErr struct{}

func (e *restartConnClosedErr) Error() string { return "restartConn: connection closed" }

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

func TestRegression_TerminalSession_RestartRoundTrip_RealStore(t *testing.T) {
	ctx := context.Background()

	home := t.TempDir()
	cwd := t.TempDir()

	const (
		projectID   = "proj-rrt"
		repoID      = "repo-rrt"
		workspaceID = "ws-rrt"
		chatID      = "chat-rrt"
	)

	worktrees := &fakeWorktreeResolver{
		ws: domain.Workspace{
			ID:           workspaceID,
			ProjectID:    projectID,
			RepoID:       repoID,
			WorktreePath: cwd,
			Provisioning: domain.WorkspaceProvisioned,
		},
	}

	dbPath := filepath.Join(home, "view.db")
	db1, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err, "open real SQLite DB for engine #1")
	sessStore1, err := storesqlite.NewFromDB[domain.TerminalSession, string](db1)
	require.NoError(t, err, "create TerminalSession store for engine #1")

	ms1 := terminal.NewSessionMetaStore(
		worktrees, fakeWorkspaces{},
		sessStore1,
		func() (string, error) { return home, nil },
	)

	eng1 := engineterminal.New()
	eng1.SetMetaStore(ms1)

	profiles1 := mocks.NewTerminalProfileStore()
	uc1 := terminal.New(eng1, profiles1, worktrees, ms1)

	sessionID, err := uc1.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "engine #1: CreateSession must succeed")

	conn1 := newRestartConn()
	attachDone1 := make(chan struct{})
	go func() {
		defer close(attachDone1)
		_ = eng1.Attach(ctx, sessionID, conn1)
	}()

	conn1.waitForData(t, func(data string) bool { return len(data) > 0 })

	require.NoError(t, eng1.Write(ctx, sessionID, []byte("echo restart-crossboundary-marker\n")))
	conn1.waitForData(t, func(data string) bool { return rrHas(data, "restart-crossboundary-marker") })

	savedCWD := cwd

	_ = conn1.Close()
	<-attachDone1

	eng1.Shutdown()

	{
		sqlDB, dbErr := db1.DB()
		require.NoError(t, dbErr, "get sql.DB handle to close engine #1 connection")
		require.NoError(t, sqlDB.Close(), "close engine #1 SQLite connection")
	}

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
		worktrees, fakeWorkspaces{},
		sessStore2,
		func() (string, error) { return home, nil },
	)

	eng2 := engineterminal.New()
	eng2.SetMetaStore(ms2)

	profiles2 := mocks.NewTerminalProfileStore()
	uc2 := terminal.New(eng2, profiles2, worktrees, ms2)

	require.NoError(t, uc2.RestorePersistedSessions(ctx),
		"engine #2: RestorePersistedSessions must succeed")

	assert.True(t, eng2.SessionExists(ctx, sessionID),
		"engine #2: session must be in registry after RestorePersistedSessions")

	state, ok := eng2.StateOf(sessionID)
	assert.True(t, ok, "engine #2: StateOf must return ok=true for restored session")
	assert.Equal(t, "suspended", state,
		"engine #2: restored session must be a suspended placeholder")

	assert.Contains(t, eng2.ListSessionsForChat(chatID), sessionID,
		"engine #2: ListSessionsForChat must include the restored session")

	conn2 := newRestartConn()
	attachDone2 := make(chan struct{})
	go func() {
		defer close(attachDone2)
		_ = eng2.Attach(ctx, sessionID, conn2)
	}()

	conn2.waitForData(t, func(data string) bool { return rrHas(data, "restart-crossboundary-marker") })

	const liveMarker = "restored-shell-live-marker"
	require.NoError(t, eng2.Write(ctx, sessionID, []byte("echo "+liveMarker+"\n")))
	conn2.waitForData(t, func(data string) bool { return rrHas(data, liveMarker) })

	require.NoError(t, eng2.Write(ctx, sessionID, []byte("[ . -ef '"+savedCWD+"' ] && echo restored-cwd-$((6*7))\n")))
	conn2.waitForData(t, func(data string) bool { return rrHas(data, "restored-cwd-42") })

	storageDir, storErr := ms2.StorageDir(ctx, chatID)
	require.NoError(t, storErr, "engine #2: must resolve storage dir for the owning chat")
	bufPath := filepath.Join(storageDir, sessionID+".buf")
	_, statErr := os.Stat(bufPath)
	assert.NoError(t, statErr, "scrollback .buf file must exist after restart-restore")

	_ = conn2.Close()
	<-attachDone2
	_ = eng2.Kill(ctx, sessionID)
	eng2.Shutdown()
}

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
			Provisioning: domain.WorkspaceProvisioned,
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
		worktrees, fakeWorkspaces{},
		sessStore,
		func() (string, error) { return home, nil },
	)

	eng := engineterminal.New()
	eng.SetMetaStore(ms)

	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, worktrees, ms)

	sessionID, err := uc.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "CreateSession must succeed")

	conn := newRestartConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sessionID, conn)
	}()

	conn.waitForData(t, func(data string) bool { return len(data) > 0 })

	eng.Shutdown()
	<-attachDone

	row, findErr := sessStore.FindByKey(ctx, sessionID)
	require.NoError(t, findErr, "FindByKey must succeed on real SQLite store")
	require.NotNil(t, row, "session row must exist in real SQLite store after Shutdown")
	assert.Equal(t, "suspended", row.State,
		"row state must be 'suspended' after Shutdown")
	assert.Equal(t, chatID, row.ChatID,
		"row must carry the correct chatID")
	assert.Equal(t, projectID, row.ProjectID,
		"row must carry the correct projectID")

	storageDir, storErr := ms.StorageDir(ctx, chatID)
	require.NoError(t, storErr, "StorageDir must resolve")
	bufPath := filepath.Join(storageDir, sessionID+".buf")
	_, statErr := os.Stat(bufPath)
	assert.NoError(t, statErr, "scrollback .buf must exist after Shutdown")
}

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
			Provisioning: domain.WorkspaceProvisioned,
		},
	}

	dbPath := filepath.Join(home, "view.db")
	db1, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	sessStore1, err := storesqlite.NewFromDB[domain.TerminalSession, string](db1)
	require.NoError(t, err)
	ms1 := terminal.NewSessionMetaStore(worktrees, fakeWorkspaces{}, sessStore1, func() (string, error) { return home, nil })

	eng1 := engineterminal.New()
	eng1.SetMetaStore(ms1)
	uc1 := terminal.New(eng1, mocks.NewTerminalProfileStore(), worktrees, ms1)

	shellID, err := uc1.CreateSession(ctx, chatID, nil)
	require.NoError(t, err, "engine #1: shell CreateSession must succeed")

	cmdID, err := eng1.CreateCommand(ctx, chatID, cwd, []string{"cat"}, nil, nil)
	require.NoError(t, err, "engine #1: CreateCommand must succeed")
	require.True(t, eng1.SessionLive(ctx, cmdID), "precondition: the vendor CLI's PTY is live before shutdown")

	eng1.Shutdown()

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
	ms2 := terminal.NewSessionMetaStore(worktrees, fakeWorkspaces{}, sessStore2, func() (string, error) { return home, nil })

	eng2 := engineterminal.New()
	eng2.SetMetaStore(ms2)
	uc2 := terminal.New(eng2, mocks.NewTerminalProfileStore(), worktrees, ms2)
	t.Cleanup(eng2.Shutdown)

	require.NoError(t, uc2.RestorePersistedSessions(ctx))

	assert.False(t, eng2.SessionExists(ctx, cmdID),
		"a command session must NOT be restored as a placeholder after a restart")
	assert.False(t, eng2.SessionLive(ctx, cmdID),
		"a command session's PTY is dead after a restart — SessionLive must say so")

	assert.True(t, eng2.SessionExists(ctx, shellID),
		"an ordinary shell session must still be restored (no regression)")
	assert.False(t, eng2.SessionLive(ctx, shellID),
		"a restored session is a placeholder: registered, but its process is dead")
}
