package terminal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestTerminalEngine_Screen_UnknownSessionReturnsUnchanged proves Screen treats an
// unknown session id the same way ScreenText treats a model-less placeholder: sessions
// die under their observers (Kill, natural exit) between a caller learning an id and
// polling it, so an unknown id must read as "nothing to see", not as an error a poller
// has to special-case.
func TestTerminalEngine_Screen_UnknownSessionReturnsUnchanged(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)

	text, gen, changed := eng.Screen("ghost", 0)
	assert.Empty(t, text)
	assert.Zero(t, gen)
	assert.False(t, changed)
}

// TestTerminalEngine_Screen_LiveSessionReturnsVisibleText proves Screen is wired end to
// end through the real engine: a real PTY session, real shell output, and the pull-style
// registry lookup + session read Screen documents in its doc comment (no Attach, no
// subscription side effects — nothing here ever calls eng.Attach).
//
// It is built on the package's existing shellsync harness (newReadyShell/runShell,
// shellsync_export_test.go) rather than a bespoke poll: runShell blocks on the session's
// real pump-progress signal until the echoed marker AND the prompt that follows it are on
// screen, which is ordered strictly after pumpStep has finished writing that PTY chunk
// into the model under s.mu. So by the time runShell returns, eng.Screen is reading
// already-settled state — no timing gap, and therefore no poll needed on this side either.
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
