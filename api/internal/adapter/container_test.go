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

	again, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })

	es := again.WorkspaceES()
	require.NotNil(t, es)
	_, err = es.ReadFrom(context.Background(), "missing", 0)
	assert.NoError(t, err)
}

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

	for _, dir := range []string{root, c.Home().State(), c.Home().Events(), c.Home().Store()} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
}

func TestNew_NarrowsTheStateDirsAnOlderBuildLeftOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	h, err := home.ResolveDir(root)
	require.NoError(t, err)
	for _, dir := range []string{h.Events(), h.Store()} {
		require.NoError(t, os.MkdirAll(dir, 0o750))
		require.NoError(t, os.Chmod(dir, 0o750))
	}
	require.NoError(t, os.Chmod(h.State(), 0o750))
	c, err := adapter.New(adapter.WithHomeDir(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	for _, dir := range []string{h.State(), h.Events(), h.Store()} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
}

func TestNew_LeavesTheModeOfAnExistingHomeRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.Mkdir(root, 0o700))
	require.NoError(t, os.Chmod(root, 0o711))

	c, err := adapter.New(adapter.WithHomeDir(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o711), info.Mode().Perm())
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

	assert.NotNil(t, c.WorkspaceES())
	assert.NotNil(t, c.WorkspaceView())
	assert.NotNil(t, c.ReviewThreadView())
	assert.NotNil(t, c.AgentChatES())
	assert.NotNil(t, c.AgentChatReadDB())
}

func TestContainer_WithHomeDir_DoesNotUseEnvHome(t *testing.T) {
	envHome := t.TempDir()
	t.Setenv("CROWBAR_HOME", envHome)

	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = os.Stat(filepath.Join(home, "state", "events", "workspace.db"))
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(envHome, "state", "events", "workspace.db"))
	require.True(t, os.IsNotExist(statErr),
		"adapter must not resolve state from CROWBAR_HOME when WithHomeDir is set")
}
