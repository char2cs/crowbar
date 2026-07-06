package terminal

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateCommand_RegistersSession(t *testing.T) {
	e := New()
	defer e.Shutdown()
	id, err := e.CreateCommand(context.Background(), "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForWorkspace("ws1"), id)
}

// TestCreateCommand_OnExitFiresOnceSessionEnds guards the resource-leak fix
// (agent.Usecase.spawnSegment's per-spawn tmp dir): the onExit callback passed
// to CreateCommand must fire exactly once reapOnDone has fully reaped a real,
// short-lived command's session — not before, and not more than once.
func TestCreateCommand_OnExitFiresOnceSessionEnds(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "true"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.Eventually(t, func() bool {
		return !e.SessionExists(ctx, id)
	}, 5*time.Second, 10*time.Millisecond, "session must be reaped after the command exits")

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&fired) == 1
	}, time.Second, 5*time.Millisecond, "onExit must fire exactly once after the session ends")

	// Give any (incorrect) duplicate invocation a chance to land, then confirm
	// the count is still exactly one.
	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must not fire more than once")
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
