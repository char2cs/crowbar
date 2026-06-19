package adapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/core/paths"
)

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
	assert.NotNil(t, third.DB)
}

func TestNew_BootsAllStores(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.WorkspaceES)
	assert.NotNil(t, c.ChatES)
	assert.NotNil(t, c.ReviewThreadES)
	assert.NotNil(t, c.DB)
}

// TestNew_OpensThreeEventStoresWithoutAgentRun pins the post-removal event-store
// layout: only workspace.db, chat.db, and review_thread.db are opened, and the
// dropped agent_run.db is never created. This guards the names-slice re-index
// that shifts ReviewThreadES from stores[3] to stores[2].
func TestNew_OpensThreeEventStoresWithoutAgentRun(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	eventsPath, err := paths.EventsAt(home)
	require.NoError(t, err)

	for _, name := range []string{"workspace.db", "chat.db", "review_thread.db"} {
		_, statErr := os.Stat(filepath.Join(eventsPath, name))
		assert.NoError(t, statErr, "expected event store %s", name)
	}
	_, statErr := os.Stat(filepath.Join(eventsPath, "agent_run.db"))
	assert.True(t, os.IsNotExist(statErr), "agent_run.db must not be created")
}

func TestClose_Idempotentish(t *testing.T) {
	home := t.TempDir()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	assert.NoError(t, c.Close())
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := adapter.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	assert.NotNil(t, c.WorkspaceES)
}
