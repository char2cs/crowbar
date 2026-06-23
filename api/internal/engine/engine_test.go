package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ReturnsContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// TestContainer_Close_ShutsDownTerminalSessions proves H15: Close must tear down
// live PTY sessions on daemon shutdown, otherwise every open terminal's shell
// (and the dev servers it spawned) is orphaned to init.
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

// TestNew_LSPFieldNonNil asserts that the LSP engine is wired into the
// container so callers can reach feature methods without nil-pointer panics.
func TestNew_LSPFieldNonNil(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c.LSP)
}

// TestContainer_Close_ShutsDownLSP proves R8: Close must tear down the LSP host
// (closing every spawned language server) on daemon shutdown, not just the
// terminal sessions — otherwise every gopls/tsserver/rust-analyzer survives the
// daemon, leaking RAM, CPU, and stdio pipe FDs across restarts. With no document
// open no server is running, so Close is a no-op here; the assertion is that it
// runs cleanly through the LSP shutdown path without panicking. The
// server-closing behaviour is covered by the engine-level
// TestShutdown_ClosesEverySpawnedServer.
func TestContainer_Close_ShutsDownLSP(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	require.NotNil(t, c.LSP)

	// Must not panic and must reach the LSP shutdown path.
	assert.NotPanics(t, c.Close)

	// Close is idempotent on the LSP host as it is on the terminal engine.
	assert.NotPanics(t, c.Close)
}
