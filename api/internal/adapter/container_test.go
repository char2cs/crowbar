package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

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

func TestReviewThreadES_Global(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	assert.NotNil(t, c.ReviewThreadES())
	assert.NotNil(t, c.ReviewThreadSS())
}

// TestAgentChatES_Global mirrors TestReviewThreadES_Global for the additive
// agentchat per-type plane (Task 9): opening the container must create the
// event log at state/events/agent_chat.db and the read-model DB at
// state/store/agent_chat.db, both non-nil.
func TestAgentChatES_Global(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	assert.NotNil(t, c.AgentChatES())
	assert.NotNil(t, c.AgentChatSS())
	assert.NotNil(t, c.AgentChatReadDB())

	_, statErr := os.Stat(filepath.Join(home, "state", "events", "agent_chat.db"))
	assert.NoError(t, statErr, "agent chat event log must exist under state/events/")
	_, statErr = os.Stat(filepath.Join(home, "state", "events", "agent_chat_snapshots.db"))
	assert.NoError(t, statErr, "agent chat snapshot store must exist under state/events/")
	_, statErr = os.Stat(filepath.Join(home, "state", "store", "agent_chat.db"))
	assert.NoError(t, statErr, "agent chat read-model db must exist under state/store/")
}

func TestClose_ClosesAll(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)

	// Touch every per-type plane so Close has real handles to checkpoint + close.
	require.NotNil(t, c.WorkspaceES())
	require.NotNil(t, c.WorkspaceSS())
	require.NotNil(t, c.WorkspaceView())
	require.NotNil(t, c.ReviewThreadES())
	require.NotNil(t, c.ReviewThreadSS())
	require.NotNil(t, c.ReviewThreadView())
	require.NotNil(t, c.AgentChatES())
	require.NotNil(t, c.AgentChatSS())
	require.NotNil(t, c.AgentChatReadDB())
	require.NotNil(t, c.GlobalView())

	require.NoError(t, c.Close())

	// A fresh container opens the same home, and its per-type event store
	// re-opens cleanly after the prior Close.
	again, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })

	es := again.WorkspaceES()
	require.NotNil(t, es)
	_, err = es.ReadFrom(context.Background(), "missing", 0)
	assert.NoError(t, err)
}

// The home is held by the boot's migration session, which opens the stores
// after it; the stores must not try to take the same lock a second time.
func TestNew_OpensAHomeItsMigrationSessionHolds(t *testing.T) {
	h, err := home.At(t.TempDir())
	require.NoError(t, err)
	sess, err := migrations.Begin(context.Background(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })

	c, err := adapter.New(adapter.WithHomeDir(h.Root()))
	require.NoError(t, err)
	require.NoError(t, c.Close())
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv(home.EnvVar, "")
	c, err := adapter.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.GlobalView())
	assert.Equal(t, filepath.Join(userHome, ".crowbar"), c.Home().Root())
}

func TestNew_CreatesAnOwnerOnlyHomeRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits not enforced on Windows")
	}
	root := filepath.Join(t.TempDir(), "home")
	c, err := adapter.New(adapter.WithHomeDir(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestNew_RejectsARelativeHome(t *testing.T) {
	_, err := adapter.New(adapter.WithHomeDir("relative/home"))
	require.ErrorContains(t, err, "must be an absolute path")
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
		filepath.Join(home, "state", "events", "agent_chat.db"),
		filepath.Join(home, "state", "store", "workspace.db"),
		filepath.Join(home, "state", "store", "review_thread.db"),
		filepath.Join(home, "state", "store", "agent_chat.db"),
		filepath.Join(home, "state", "view.db"),
	} {
		_, statErr := os.Stat(p)
		require.NoError(t, statErr, "expected %s to exist under the isolated home", p)
	}

	// The new per-type handles must be live.
	assert.NotNil(t, c.WorkspaceES())
	assert.NotNil(t, c.WorkspaceView())
	assert.NotNil(t, c.ReviewThreadView())
	assert.NotNil(t, c.AgentChatES())
	assert.NotNil(t, c.AgentChatReadDB())
}

// TestContainer_WithHomeDir_DoesNotUseEnvHome guards the decision-14 isolation
// requirement: WithHomeDir must root every state subtree at the resolved
// home, never CROWBAR_HOME; otherwise the per-type DBs would leak into
// the CROWBAR_HOME root instead of the temp home.
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
	// ...and NOT under the CROWBAR_HOME env root.
	_, statErr := os.Stat(filepath.Join(envHome, "state", "events", "workspace.db"))
	require.True(t, os.IsNotExist(statErr),
		"adapter must not resolve state from CROWBAR_HOME when WithHomeDir is set")
}
