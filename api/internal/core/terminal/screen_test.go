package terminal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

func TestTerminalEngine_Screen_UnknownSessionReturnsUnchanged(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)

	text, gen, changed := eng.Screen("ghost", 0)
	assert.Empty(t, text)
	assert.Zero(t, gen)
	assert.False(t, changed)
}

func TestTerminalEngine_Screen_LiveSessionReturnsVisibleText(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	dir := t.TempDir()

	sid := newReadyShell(t, eng, "ws-1", dir)

	const marker = "SCREENTEST-4f2a9c"
	runShell(t, eng, sid, "echo "+marker)

	text, gen, changed := eng.Screen(sid, 0)
	assert.True(t, changed, "a session with real output must report changed at since=0")
	assert.Contains(t, text, marker, "Screen must return the text the session's own screen shows")
	assert.NotZero(t, gen, "a live session with real output must report a nonzero generation")

	require.NoError(t, eng.Kill(context.Background(), sid))
}
