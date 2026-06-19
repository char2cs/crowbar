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

	es, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	require.NotNil(t, es)

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

	view, err := c.WorkspaceView("p1", "r1", "w1")
	require.NoError(t, err)
	require.NotNil(t, view)

	_, statErr = os.Stat(dbPath)
	assert.NoError(t, statErr, "view.db must exist after WorkspaceView")
}

func TestWorkspaceES_CachedSecondCall(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	first, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	second, err := c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
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

	_, err = c.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	_, err = c.WorkspaceView("p1", "r1", "w1")
	require.NoError(t, err)

	require.NoError(t, c.Close())

	// The lock is released, so a fresh container can open the same home.
	again, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })

	// And the per-entity stores re-open cleanly after the prior Close.
	es, err := again.WorkspaceES("p1", "r1", "w1")
	require.NoError(t, err)
	assert.NotNil(t, es)
	_, err = es.ReadFrom(context.Background(), "missing", 0)
	assert.NoError(t, err)
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

	_, err = c.WorkspaceES("p1", "r1", "w1")
	assert.Error(t, err)

	_, err = c.WorkspaceView("p1", "r1", "w1")
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
