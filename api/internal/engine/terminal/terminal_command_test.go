package terminal

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateCommand_RegistersSession(t *testing.T) {
	e := New()
	defer e.Shutdown()
	id, err := e.CreateCommand(context.Background(), "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ())
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForWorkspace("ws1"), id)
}

func TestWithTerminalDefaults_InjectsTERMWhenAbsent(t *testing.T) {
	// A command session started with an env lacking TERM ends up with the default.
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "TERM=xterm-256color")
	require.Contains(t, got, "COLORTERM=truecolor")
	// Does not override a caller-provided TERM.
	got = withTerminalDefaults([]string{"TERM=screen"})
	require.Contains(t, got, "TERM=screen")
	require.NotContains(t, got, "TERM=xterm-256color")
}
