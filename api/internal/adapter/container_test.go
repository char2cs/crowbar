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

func TestClose_ClosesAllAndLock(t *testing.T) {
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
	require.NotNil(t, c.AgentRunnerES())
	require.NotNil(t, c.AgentRunnerSS())
	require.NotNil(t, c.AgentRunnerReadDB())
	require.NotNil(t, c.GlobalView())

	require.NoError(t, c.Close())

	// The lock is released, so a fresh container can open the same home and its
	// per-type event store re-opens cleanly after the prior Close.
	again, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })

	es := again.WorkspaceES()
	require.NotNil(t, es)
	_, err = es.ReadFrom(context.Background(), "missing", 0)
	assert.NoError(t, err)
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
