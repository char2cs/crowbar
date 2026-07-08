package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

func storagesDir(
	home string,
	projectID string,
	repoID string,
	wsID string,
) string {
	return filepath.Join(
		home,
		"projects",
		projectID,
		repoID,
		"workspaces",
		wsID,
		"storages",
	)
}

func TestWorkspaceES_LazyOpenCreatesEventStreamDB(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	dbPath := filepath.Join(storagesDir(home, "p1", "r1", "w1"), "event_stream.db")
	_, statErr := os.Stat(dbPath)
	require.True(t, os.IsNotExist(statErr), "event_stream.db must not exist before first access")

	es, release, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	require.NotNil(t, es)
	t.Cleanup(release)

	_, statErr = os.Stat(dbPath)
	assert.NoError(t, statErr, "event_stream.db must exist after WorkspaceES")
}

func TestWorkspaceView_LazyOpenCreatesViewDB(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	dbPath := filepath.Join(storagesDir(home, "p1", "r1", "w1"), "view.db")
	_, statErr := os.Stat(dbPath)
	require.True(t, os.IsNotExist(statErr), "view.db must not exist before first access")

	view, release, err := c.WorkspaceView("p1", "r1", "w1")
	require.NoError(t, err)
	require.NotNil(t, view)
	t.Cleanup(release)

	_, statErr = os.Stat(dbPath)
	assert.NoError(t, statErr, "view.db must exist after WorkspaceView")
}

func TestWorkspaceES_CachedSecondCall(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	first, releaseFirst, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	t.Cleanup(releaseFirst)
	second, releaseSecond, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	t.Cleanup(releaseSecond)
	assert.Same(t, first, second)
}

func TestGlobalView_HoldsProfilesAndSettings(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	view := c.GlobalView()
	require.NotNil(t, view)

	dbPath := filepath.Join(home, "state", "view.db")
	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr, "global view.db must exist under state/")
}

// TestGlobalView_UsesReadPool guards spec decision 12: the global view.db is a
// read-model/view DB, so it MUST be opened with the multi-conn read pool, never
// the single-conn OpenDB (the documented head-of-line "original wedge"). We pin
// its MaxOpenConnections to the same value a fresh OpenReadPoolDB reports, so the
// assertion tracks the read-pool sizing instead of a magic number.
func TestGlobalView_UsesReadPool(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	globalSQLDB, err := c.GlobalView().DB()
	require.NoError(t, err)

	ref, err := storesqlite.OpenReadPoolDB(filepath.Join(t.TempDir(), "ref.db"))
	require.NoError(t, err)
	refSQLDB, err := ref.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = refSQLDB.Close() })

	assert.Equal(t, refSQLDB.Stats().MaxOpenConnections, globalSQLDB.Stats().MaxOpenConnections,
		"global view.db must be opened with the read pool (decision 12)")
	assert.Greater(t, globalSQLDB.Stats().MaxOpenConnections, 1,
		"global view.db must be multi-conn, not the single-conn wedge DB")
}

func TestChatAndReviewThreadES_Global(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	assert.NotNil(t, c.ChatES())
	assert.NotNil(t, c.ReviewThreadES())
}

func TestClose_ClosesAllAndLock(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	_, releaseES, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	releaseES()
	_, releaseView, err := c.WorkspaceView("p1", "r1", "w1")
	require.NoError(t, err)
	releaseView()

	require.NoError(t, c.Close())

	// The lock is released, so a fresh container can open the same home.
	again, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })

	// And the per-entity stores re-open cleanly after the prior Close.
	es, releaseAgain, err := again.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	t.Cleanup(releaseAgain)
	assert.NotNil(t, es)
	_, err = es.ReadFrom(context.Background(), "missing", 0)
	assert.NoError(t, err)
}

func TestRegression_AccessorsRefuseReopenAfterClose(t *testing.T) {
	// A detached good-path-async goroutine (runAsync on context.WithoutCancel)
	// could lazily re-open a per-entity DB just after Close, re-creating its
	// storages dir and racing teardown. The accessors must refuse once closed.
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	require.NoError(t, c.Close())

	_, _, esErr := c.WorkspaceES("p1", "r1", "w1")
	require.ErrorIs(t, esErr, adapter.ErrClosed)
	_, _, viewErr := c.WorkspaceView("p1", "r1", "w1")
	require.ErrorIs(t, viewErr, adapter.ErrClosed)

	// The closed accessor must NOT have re-created the entity storages dir.
	assert.NoDirExists(t, filepath.Join(home, "projects", "p1", "r1", "workspaces", "w1", "storages"))
}

func TestWorkspaceES_MkdirError(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	// Plant a regular file where the projects subtree would need a directory, so
	// MkdirAll of the storages dir fails.
	require.NoError(t, os.MkdirAll(filepath.Join(home, "projects"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, "projects", "p1"), []byte("x"), 0o600))

	_, _, err = c.WorkspaceES("p1", "r1", "w1")
	assert.Error(t, err)

	_, _, err = c.WorkspaceView("p1", "r1", "w1")
	assert.Error(t, err)
}

func TestRegression_StateDirSingleInstanceLock(t *testing.T) {
	home := t.TempDir()

	first, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	second, err := adapter.New(adapter.WithHomeDir(home))
	require.Error(t, err)
	assert.Nil(t, second)
	assert.Contains(t, strings.ToLower(err.Error()), "lock")

	require.NoError(t, first.Close())

	third, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = third.Close() })
	assert.NotNil(t, third.GlobalView())
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := adapter.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.GlobalView())
}

func TestContainer_WithHomeDir_IsolatesAllState(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	// Every per-type DB file must live under `home`, never under the real ~/.crowbar.
	for _, p := range []string{
		filepath.Join(home, "state", "events", "workspace.db"),
		filepath.Join(home, "state", "events", "review_thread.db"),
		filepath.Join(home, "state", "store", "workspace.db"),
		filepath.Join(home, "state", "store", "review_thread.db"),
		filepath.Join(home, "state", "view.db"),
	} {
		_, statErr := os.Stat(p)
		require.NoError(t, statErr, "expected %s to exist under the isolated home", p)
	}

	// The new per-type handles must be live.
	assert.NotNil(t, c.WorkspaceEventStore())
	assert.NotNil(t, c.WorkspaceStoreDB())
	assert.NotNil(t, c.ReviewThreadView())
}

// TestContainer_WithHomeDir_DoesNotUseEnvHome guards the decision-14 isolation
// requirement: WithHomeDir must root every state subtree at the resolved
// crowbarHome, NEVER the home-agnostic paths.Events()/Store()/State() (which
// resolve from CROWBAR_HOME). If the adapter used the env-blind accessors, the
// per-type DBs would leak into the CROWBAR_HOME root instead of the temp home.
func TestContainer_WithHomeDir_DoesNotUseEnvHome(t *testing.T) {
	envHome := t.TempDir()
	t.Setenv("CROWBAR_HOME", envHome)

	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	// State lands under the explicit WithHomeDir root...
	_, err = os.Stat(filepath.Join(home, "state", "events", "workspace.db"))
	require.NoError(t, err)
	// ...and NOT under the CROWBAR_HOME env root that paths.Events() would pick.
	_, statErr := os.Stat(filepath.Join(envHome, "state", "events", "workspace.db"))
	require.True(t, os.IsNotExist(statErr),
		"adapter must not resolve state from CROWBAR_HOME when WithHomeDir is set")
}
