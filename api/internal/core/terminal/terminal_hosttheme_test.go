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

const bgReportPrefixLen = 23

func oscQueryScript(outPath string) []string {
	const bgQuery = "\x1b]11;?\x07"
	return []string{
		"/bin/sh", "-c",
		"stty raw -echo; printf %s '" + bgQuery + "'; " +
			"head -c " + strconv.Itoa(bgReportPrefixLen) + " > " + outPath,
	}
}

func TestRegression_CommandSessionAnswersOSC11WithHostThemeAtBirth(t *testing.T) {
	e := New()
	defer e.Shutdown()

	e.SetHostTheme(color.RGBA{R: 0xfa, G: 0xf9, B: 0xf5, A: 0xff}, color.RGBA{R: 0x14, G: 0x14, B: 0x14, A: 0xff})

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func(Exit) { close(exits) })
	require.NoError(t, err)

	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "rgb:",
		"the command must have received an OSC 11 default-background report")
	require.Contains(t, string(reply), "fafa/f9f9/f5f5",
		"a session must be born already answering with the host theme, not x/vt's black")
}

func TestCommandSessionWithoutHostTheme_KeepsEmulatorDefault(t *testing.T) {
	e := New()
	defer e.Shutdown()

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func(Exit) { close(exits) })
	require.NoError(t, err)
	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "rgb:0000/0000/0000",
		"with no host theme pushed the emulator default (black) must still be what answers")
}

func TestSetHostTheme_LastPushWins(t *testing.T) {
	e := New()
	defer e.Shutdown()

	e.SetHostTheme(color.RGBA{R: 0xfa, G: 0xf9, B: 0xf5, A: 0xff}, color.White)
	e.SetHostTheme(color.RGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}, color.White)

	out := filepath.Join(t.TempDir(), "reply")
	exits := make(chan struct{})
	_, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		oscQueryScript(out), os.Environ(), func(Exit) { close(exits) })
	require.NoError(t, err)
	<-exits

	reply, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(reply), "1e1e/1e1e/1e1e")
}
