package terminal_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

const shellPrompt = terminal.ShellPromptForTest

func pinShell(t *testing.T) { t.Helper(); terminal.PinShellForTest(t) }

func runShell(t *testing.T, eng terminal.Engine, sid, cmd string) {
	t.Helper()
	terminal.RunShellForTest(t, eng, sid, cmd)
}

func startForeground(t *testing.T, eng terminal.Engine, sid string) {
	t.Helper()
	terminal.StartForegroundForTest(t, eng, sid)
}

func newReadyShell(t *testing.T, eng terminal.Engine, chatID, dir string) string {
	t.Helper()
	return terminal.NewReadyShellForTest(t, eng, chatID, dir)
}
