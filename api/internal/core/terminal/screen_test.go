package terminal_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
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

	sid := newReadyShell(t, eng, "chat-1", dir)

	const marker = "SCREENTEST-4f2a9c"
	runShell(t, eng, sid, "echo "+marker)

	text, gen, changed := eng.Screen(sid, 0)
	assert.True(t, changed, "a session with real output must report changed at since=0")
	assert.Contains(t, text, marker, "Screen must return the text the session's own screen shows")
	assert.NotZero(t, gen, "a live session with real output must report a nonzero generation")

	require.NoError(t, eng.Kill(context.Background(), sid))
}

func TestTerminalEngine_ScreenState_UnknownSessionIsNotFound(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)

	_, err := screenStateReader(t, eng).ScreenState("ghost")
	require.ErrorIs(t, err, terminal.ErrSessionNotFound)
}

func screenStateReader(t *testing.T, eng terminal.Engine) terminal.ScreenStateReader {
	t.Helper()
	reader, ok := eng.(terminal.ScreenStateReader)
	require.True(t, ok, "the engine exposes the atomic screen read")
	return reader
}

func TestTerminalEngine_ScreenState_ModelWithoutStateIsUnavailable(t *testing.T) {
	pinShell(t)
	restore := session.SetNewModelForTest(func(c, r, _ int) (model.TerminalModel, model.Serializer) {
		return &degradedModel{cols: c, rows: r}, degradedSerializer{}
	})
	defer restore()
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()

	sid, err := eng.Create(context.Background(), "chat-nostate", t.TempDir(), nil)
	require.NoError(t, err)

	_, err = screenStateReader(t, eng).ScreenState(sid)
	require.ErrorIs(t, err, terminal.ErrScreenStateUnavailable)
}

func TestTerminalEngine_ScreenState_LiveSessionReportsItsScreen(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	sid := newReadyShell(t, eng, "chat-1", t.TempDir())
	reader := screenStateReader(t, eng)

	const marker = "STATETEST-7b1e30"
	runShell(t, eng, sid, "echo "+marker)

	st, err := reader.ScreenState(sid)
	require.NoError(t, err)
	assert.Contains(t, st.Text, marker)
	text, _, _ := eng.Screen(sid, 0)
	assert.Equal(t, text, st.Text, "same text Screen returns")
	assert.Less(t, st.CursorRow, strings.Count(st.Text, "\n")+1, "cursor row is a row of the visible grid")
	assert.GreaterOrEqual(t, st.CursorCol, 0)

	require.NoError(t, eng.Kill(context.Background(), sid))
}
