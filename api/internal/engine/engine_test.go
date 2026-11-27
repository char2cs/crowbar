package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestNew_ReturnsContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNew_SpawnedCLIsRunTheDaemonsOwnExecutable(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	exe, err := os.Executable()
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(exe)
	require.NoError(t, err)
	assert.Equal(t, want, c.CrowbarBin)
}

func TestLiveExecutable_FallsBackToProcWhenTheBinaryIsGone(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "crowbar")
	got := liveExecutable(gone+" (deleted)", 4242)
	if runtime.GOOS == "linux" {
		assert.Equal(t, fmt.Sprintf("/proc/%d/exe", 4242), got)
		return
	}
	assert.Equal(t, gone, got)
}

func TestContainer_Close_ShutsDownTerminalSessions(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	id, err := c.Terminal.Create(context.Background(), "ws1", t.TempDir(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.Len(t, c.Terminal.ListSessions(), 1)

	c.Close()

	assert.Empty(t, c.Terminal.ListSessions(),
		"Close must terminate live PTY sessions so no shell child is orphaned on shutdown")
}

func TestNew_LSPFieldNonNil(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c.LSP)
}

func TestContainer_Close_ShutsDownLSP(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	require.NotNil(t, c.LSP)

	assert.NotPanics(t, c.Close)

	assert.NotPanics(t, c.Close)
}

type closeSpyAgents struct {
	engineagents.Agents
	closed int
}

func (s *closeSpyAgents) Close() {
	s.closed++
	s.Agents.Close()
}

func TestRegression_ContainerCloseJoinsTheAgentsEngine(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	spy := &closeSpyAgents{Agents: c.Agents}
	c.Agents = spy

	c.Close()

	assert.Equal(t, 1, spy.closed,
		"engine Close must join the agents engine's background model discovery")
}
