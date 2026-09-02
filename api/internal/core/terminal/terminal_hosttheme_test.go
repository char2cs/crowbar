//go:build !windows

package terminal

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// bgReportPrefixLen is the length of an OSC 11 default-background report up to but NOT
// including its terminator: ESC ] 1 1 ; r g b : XXXX / XXXX / XXXX. The emulator's reply
// shape is pinned by the model's own tests (TestSetDefaultColors_ReportedViaOSC11Query
// asserts "\x1b]11;rgb:ffff/ffff/ffff"), and stopping one byte short of the end is what
// makes reading a fixed count safe: whether the reply closes with BEL or with ST, the first
// 23 bytes are the same and carry the whole colour.
const bgReportPrefixLen = 23

// oscQueryScript builds a /bin/sh -c script that asks the terminal for its default
// background colour the way a TUI does at startup (OSC 11) and records the answer at
// outPath. It is the reduced form of what codex and Claude Code do on the first frame.
//
// `head -c` reads the reply directly off the tty and exits, which tears the PTY down and
// makes the COMMAND'S EXIT the test's read barrier — no clock anywhere. It deliberately
// does NOT pipe through `tr` to find the terminator: tr block-buffers into a pipe, so the
// decoded reply would sit in its stdio buffer, unflushed, and the command would never exit.
//
// The ESC/BEL of the query itself are literal bytes from Go rather than printf escapes,
// because `\033` is not portable across the shells that may back /bin/sh.
func oscQueryScript(outPath string) []string {
	const bgQuery = "\x1b]11;?\x07"
	return []string{
		"/bin/sh", "-c",
		"stty raw -echo; printf %s '" + bgQuery + "'; " +
			"head -c " + strconv.Itoa(bgReportPrefixLen) + " > " + outPath,
	}
}

// TestRegression_CommandSessionAnswersOSC11WithHostThemeAtBirth is the black-box guard for
// the Codex light/dark bug: a vendor CLI that detects its theme by querying the background
// colour at startup got x/vt's hardcoded black, because the host theme only ever reached a
// session from the FRONTEND on attach — necessarily after CreateCommand had already exec'd
// the process.
//
// Claude Code masked it by also subscribing to DEC 2031, so the late attach-time push
// notified it and it re-queried; codex 0.146.0 has no 2031 support at all, so the answer it
// got at birth was the only one it would ever see and the wrong polarity latched for the life
// of the process.
//
// The command here is the reduced form of that CLI: it queries at startup and nothing else.
func TestRegression_CommandSessionAnswersOSC11WithHostThemeAtBirth(t *testing.T) {
	e := New()
	defer e.Shutdown()

	// Crowbar's light background — the value the FE resolves from --background in light mode.
	e.SetHostTheme(color.RGBA{R: 0xfa, G: 0xf9, B: 0xf5, A: 0xff}, color.RGBA{R: 0x14, G: 0x14, B: 0x14, A: 0xff})

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func() { close(exits) })
	require.NoError(t, err)

	// The command exits as soon as it has the reply, so its exit IS the read barrier. No
	// deadline is needed to keep this from hanging: the model ALWAYS answers an OSC 11
	// query — before the fix it answered black — so a regression fails the assertion below
	// rather than blocking here.
	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "rgb:",
		"the command must have received an OSC 11 default-background report")
	require.Contains(t, string(reply), "fafa/f9f9/f5f5",
		"a session must be born already answering with the host theme, not x/vt's black")
}

// TestCommandSessionWithoutHostTheme_KeepsEmulatorDefault pins the untouched fallback: a
// daemon that has never been told the host theme (nothing has pushed one yet) must leave the
// emulator's own default in place rather than seeding some invented colour.
func TestCommandSessionWithoutHostTheme_KeepsEmulatorDefault(t *testing.T) {
	e := New()
	defer e.Shutdown()

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func() { close(exits) })
	require.NoError(t, err)
	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "rgb:0000/0000/0000",
		"with no host theme pushed the emulator default (black) must still be what answers")
}

// TestSetHostTheme_LastPushWins covers the daemon-side record itself: the host theme is
// ambient state shared by every session, so the newest push is what a later-born session
// inherits — which is what makes a theme switch followed by a new agent chat correct.
func TestSetHostTheme_LastPushWins(t *testing.T) {
	e := New()
	defer e.Shutdown()

	e.SetHostTheme(color.RGBA{R: 0xfa, G: 0xf9, B: 0xf5, A: 0xff}, color.White)
	e.SetHostTheme(color.RGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}, color.White)

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func() { close(exits) })
	require.NoError(t, err)
	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "1e1e/1e1e/1e1e")
}
