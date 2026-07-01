package terminal_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func newTerminalUsecase(
	t *testing.T,
) (
	*mocks.TerminalEngine,
	*mocks.TerminalProfileStore,
	*mocks.WorkspaceSyncer,
	terminal.Usecase,
) {
	t.Helper()
	eng := mocks.NewTerminalEngine()
	profiles := mocks.NewTerminalProfileStore()
	syncer := mocks.NewWorkspaceSyncer()
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	uc := terminal.New(eng, profiles, syncer, nil)
	return eng, profiles, syncer, uc
}

func TestTerminalUsecase_CreateSession_ResolvesWorkspaceDir(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	var gotDir string
	eng.CreateFn = func(_ context.Context, _, dir string, _ *domain.TerminalProfile) (string, error) {
		gotDir = dir
		return "sess1", nil
	}

	id, err := uc.CreateSession(ctx, "w1", nil)
	require.NoError(t, err)
	assert.Equal(t, "sess1", id)
	assert.Equal(t, "/repo/x", gotDir)
}

func TestTerminalUsecase_CreateSession_WorkspaceError(t *testing.T) {
	eng, _, syncer, uc := newTerminalUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	eng.CreateFn = func(_ context.Context, _, _ string, _ *domain.TerminalProfile) (string, error) {
		return "", nil
	}

	_, err := uc.CreateSession(ctx, "w1", nil)
	assert.Error(t, err)
}

func TestTerminalUsecase_CreateSession_EngineError(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	eng.CreateFn = func(_ context.Context, _, _ string, _ *domain.TerminalProfile) (string, error) {
		return "", errors.New("boom")
	}

	_, err := uc.CreateSession(ctx, "w1", nil)
	assert.Error(t, err)
}

func TestTerminalUsecase_KillSession(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	var killed string
	eng.KillFn = func(_ context.Context, id string) error {
		killed = id
		return nil
	}

	require.NoError(t, uc.KillSession(ctx, "sess1"))
	assert.Equal(t, "sess1", killed)
}

func TestTerminalUsecase_KillSession_Error(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	eng.KillFn = func(_ context.Context, _ string) error { return errors.New("boom") }

	assert.Error(t, uc.KillSession(ctx, "sess1"))
}

func TestTerminalUsecase_ListProfiles(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	_ = profiles.Save(ctx, domain.TerminalProfile{ID: "p1", Name: "a"})

	list, err := uc.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestTerminalUsecase_ListProfiles_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.FindAllErr = errors.New("boom")

	_, err := uc.ListProfiles(ctx)
	assert.Error(t, err)
}

func TestTerminalUsecase_CreateProfile_MintsID(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	got, err := uc.CreateProfile(ctx, domain.TerminalProfile{Name: "a"})
	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Len(t, profiles.Saved, 1)
}

func TestTerminalUsecase_CreateProfile_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.SaveErr = errors.New("boom")

	_, err := uc.CreateProfile(ctx, domain.TerminalProfile{Name: "a"})
	assert.Error(t, err)
}

func TestTerminalUsecase_UpdateProfile(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	_ = profiles.Save(ctx, domain.TerminalProfile{ID: "p1", Name: "a"})

	got, err := uc.UpdateProfile(ctx, domain.TerminalProfile{ID: "p1", Name: "b"})
	require.NoError(t, err)
	assert.Equal(t, "b", got.Name)
}

func TestTerminalUsecase_UpdateProfile_MissingID(t *testing.T) {
	_, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	_, err := uc.UpdateProfile(ctx, domain.TerminalProfile{Name: "b"})
	assert.Error(t, err)
}

func TestTerminalUsecase_UpdateProfile_SaveError(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.SaveErr = errors.New("boom")

	_, err := uc.UpdateProfile(ctx, domain.TerminalProfile{ID: "p1", Name: "b"})
	assert.Error(t, err)
}

func TestTerminalUsecase_DeleteProfile(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	require.NoError(t, uc.DeleteProfile(ctx, "p1"))
	assert.Equal(t, []string{"p1"}, profiles.Deleted)
}

func TestTerminalUsecase_DeleteProfile_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.DeleteErr = errors.New("boom")

	assert.Error(t, uc.DeleteProfile(ctx, "p1"))
}

// ---------------------------------------------------------------------------
// Phase 3: RestorePersistedSessions tests (TDD — written first)
// ---------------------------------------------------------------------------

// newMetaStoreForTest builds a real sessionMetaStoreImpl backed by in-memory
// fakes for use in restart-restore tests.
func newMetaStoreForTest(
	t *testing.T,
	wsRepo terminal.WorkspaceRepo,
	sessStore *fakeSessionStore,
	home string,
) engineterminal.SessionMetaStore {
	t.Helper()
	return terminal.NewSessionMetaStore(wsRepo, sessStore, func() (string, error) { return home, nil })
}

// TestRestorePersistedSessions_RestoresPlaceholder is the restart round-trip:
// write a session row + .buf (simulating a pre-restart suspend), build a fresh
// engine + metastore, call RestorePersistedSessions, and verify the session is
// a suspended placeholder in the new engine with the scrollback loadable.
func TestRestorePersistedSessions_RestoresPlaceholder(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sessionID := "restart-sess-1"

	wsRepo := &fakeWorkspaceRepo{
		ws: domain.Workspace{
			ID:        "ws-restart",
			ProjectID: "proj-restart",
			RepoID:    "repo-restart",
		},
	}
	sessStore := newFakeSessionStore()
	ms := newMetaStoreForTest(t, wsRepo, sessStore, home)

	// Pre-populate as if a daemon had previously written this session.
	sessStore.rows[sessionID] = domain.TerminalSession{
		SessionID:   sessionID,
		WorkspaceID: "ws-restart",
		CWD:         t.TempDir(),
		Shell:       "/bin/sh",
		ProfileID:   "",
		State:       "suspended",
	}

	// Write scrollback file to the storage dir that metaStore will resolve.
	storageDir, err := ms.StorageDir(ctx, "ws-restart")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(storageDir, 0o755))
	scrollback := []byte("pre-restart-output")
	require.NoError(t, os.WriteFile(filepath.Join(storageDir, sessionID+".buf"), scrollback, 0o644))

	// Fresh engine + usecase simulating daemon restart. The maintenance loop
	// won't interfere: it only acts on live (detached/idle) sessions; a fresh
	// engine with only placeholders has nothing to sweep.
	freshEng := engineterminal.New()
	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(freshEng, profiles, wsRepo, ms)

	require.NoError(t, uc.RestorePersistedSessions(ctx))

	// Session must be registered as a suspended placeholder.
	assert.True(t, freshEng.SessionExists(ctx, sessionID), "session must be in registry after restore")
	state, ok := freshEng.StateOf(sessionID)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state, "restored session must be a placeholder (suspended)")

	// Cleanup.
	require.NoError(t, freshEng.Kill(ctx, sessionID))
}

// TestRestorePersistedSessions_OrphanDeleted verifies that when a session row's
// workspace can no longer be resolved (deleted workspace), the row is deleted from
// the store and no placeholder is registered in the engine.
func TestRestorePersistedSessions_OrphanDeleted(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sessionID := "orphan-sess-1"

	// Workspace repo that always returns "not found".
	wsRepo := &fakeWorkspaceRepo{err: errors.New("workspace not found")}
	sessStore := newFakeSessionStore()
	ms := newMetaStoreForTest(t, wsRepo, sessStore, home)

	// Pre-populate an orphaned row.
	sessStore.rows[sessionID] = domain.TerminalSession{
		SessionID:   sessionID,
		WorkspaceID: "ws-deleted",
		State:       "suspended",
	}

	eng := mocks.NewTerminalEngine()
	var loadedIDs []string
	eng.LoadPlaceholderFn = func(_ context.Context, m engineterminal.SessionMeta, _ []byte) error {
		loadedIDs = append(loadedIDs, m.SessionID)
		return nil
	}
	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, wsRepo, ms)

	require.NoError(t, uc.RestorePersistedSessions(ctx))

	// The orphan row must be deleted.
	_, stillPresent := sessStore.rows[sessionID]
	assert.False(t, stillPresent, "orphan row must be deleted from the store")

	// No placeholder must be registered.
	assert.NotContains(t, loadedIDs, sessionID, "no placeholder must be loaded for orphaned session")
}

// TestRestorePersistedSessions_BestEffort verifies that a per-row error (engine
// returns error for one session) does not abort the whole restore: the remaining
// sessions are still processed and RestorePersistedSessions returns nil.
func TestRestorePersistedSessions_BestEffort(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	wsRepo := &fakeWorkspaceRepo{ws: domain.Workspace{ID: "ws-1", ProjectID: "p", RepoID: "r"}}
	sessStore := newFakeSessionStore()
	ms := newMetaStoreForTest(t, wsRepo, sessStore, home)

	sessStore.rows["sess-fail"] = domain.TerminalSession{SessionID: "sess-fail", WorkspaceID: "ws-1", State: "suspended"}
	sessStore.rows["sess-ok"] = domain.TerminalSession{SessionID: "sess-ok", WorkspaceID: "ws-1", State: "suspended"}

	eng := mocks.NewTerminalEngine()
	var loaded []string
	eng.LoadPlaceholderFn = func(_ context.Context, m engineterminal.SessionMeta, _ []byte) error {
		if m.SessionID == "sess-fail" {
			return errors.New("engine error")
		}
		loaded = append(loaded, m.SessionID)
		return nil
	}
	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, wsRepo, ms)

	// Must not return an error despite one row failing.
	err := uc.RestorePersistedSessions(ctx)
	require.NoError(t, err, "RestorePersistedSessions must return nil even when a row fails")

	// The succeeding session must be registered.
	assert.Contains(t, loaded, "sess-ok", "successful session must be loaded despite peer failure")
}

// TestRestorePersistedSessions_CapsToMaxAndEvictsOldest verifies that when more
// session rows are persisted than the engine's global ceiling allows, restore
// reloads only the cap (newest by LastActiveAt) and DELETES the excess rows plus
// their .buf files (oldest first), bounding both memory and disk on restart.
func TestRestorePersistedSessions_CapsToMaxAndEvictsOldest(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	wsRepo := &fakeWorkspaceRepo{ws: domain.Workspace{ID: "ws-1", ProjectID: "p", RepoID: "r"}}
	sessStore := newFakeSessionStore()
	ms := newMetaStoreForTest(t, wsRepo, sessStore, home)

	storageDir, err := ms.StorageDir(ctx, "ws-1")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(storageDir, 0o755))

	limit := engineterminal.MaxTotalSessions()
	total := limit + 2 // two over the cap
	base := time.Now()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("sess-%03d", i) // higher i ⇒ more recent
		sessStore.rows[id] = domain.TerminalSession{
			SessionID:    id,
			WorkspaceID:  "ws-1",
			State:        "suspended",
			LastActiveAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(storageDir, id+".buf"), []byte("x"), 0o644))
	}

	eng := mocks.NewTerminalEngine()
	var loaded []string
	eng.LoadPlaceholderFn = func(_ context.Context, m engineterminal.SessionMeta, _ []byte) error {
		loaded = append(loaded, m.SessionID)
		return nil
	}
	profiles := mocks.NewTerminalProfileStore()
	uc := terminal.New(eng, profiles, wsRepo, ms)

	require.NoError(t, uc.RestorePersistedSessions(ctx))

	// Only the cap restored; the two oldest are not.
	assert.Len(t, loaded, limit, "must restore at most the global cap")
	assert.NotContains(t, loaded, "sess-000", "oldest must not be restored")
	assert.NotContains(t, loaded, "sess-001", "second-oldest must not be restored")

	// The two oldest rows + their .buf files are deleted from disk.
	_, ok0 := sessStore.rows["sess-000"]
	_, ok1 := sessStore.rows["sess-001"]
	assert.False(t, ok0, "oldest over-cap row must be deleted")
	assert.False(t, ok1, "second-oldest over-cap row must be deleted")
	assert.NoFileExists(t, filepath.Join(storageDir, "sess-000.buf"))
	assert.NoFileExists(t, filepath.Join(storageDir, "sess-001.buf"))

	// The newest row survives and is restored.
	newest := fmt.Sprintf("sess-%03d", total-1)
	assert.Contains(t, loaded, newest, "newest row must be restored")
	_, okNewest := sessStore.rows[newest]
	assert.True(t, okNewest, "newest row must remain on disk")
}

// TestRestorePersistedSessions_NilMetaStore verifies that RestorePersistedSessions
// is a no-op when no metaStore is configured (returns nil immediately).
func TestRestorePersistedSessions_NilMetaStore(t *testing.T) {
	eng := mocks.NewTerminalEngine()
	profiles := mocks.NewTerminalProfileStore()
	syncer := mocks.NewWorkspaceSyncer()
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	uc := terminal.New(eng, profiles, syncer, nil) // no metaStore

	err := uc.RestorePersistedSessions(context.Background())
	assert.NoError(t, err)
}
