package terminal_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// Thin aliases over the shared, clock-free shell-synchronisation harness. The implementation
// lives in shellsync_export_test.go (package terminal) so that the internal hardening/cover
// tests and this external suite share ONE definition of "the shell is ready" rather than
// drifting into two.
//
// Read that file for the reasoning. In short: these tests drive real PTYs and real shells, so
// a duration is never an answer to "has the shell got there yet?" — it is a guess about
// fork/exec speed. Every helper here blocks on a real signal instead: the shell's own prompt,
// the pump's progress seam, or the kernel's foreground-process-group verdict.

const shellPrompt = terminal.ShellPromptForTest

// pinShell pins the shell to /bin/sh with a known prompt and no rc file, so its output is a
// protocol rather than a lottery. Must be called before creating any session.
func pinShell(t *testing.T) { t.Helper(); terminal.PinShellForTest(t) }

// runShell runs cmd and blocks until it has finished AND the prompt has returned, leaving the
// session quiescent with no output in flight.
func runShell(t *testing.T, eng terminal.Engine, sid, cmd string) {
	t.Helper()
	terminal.RunShellForTest(t, eng, sid, cmd)
}

// startForeground starts a long-running foreground child and blocks until the kernel agrees
// it owns the terminal (the same TIOCGPGRP measure the maintenance sweep's idle gate reads).
func startForeground(t *testing.T, eng terminal.Engine, sid string) {
	t.Helper()
	terminal.StartForegroundForTest(t, eng, sid)
}

// newReadyShell creates a session with a pinned shell and blocks until it is at its prompt.
func newReadyShell(t *testing.T, eng terminal.Engine, chatID, dir string) string {
	t.Helper()
	return terminal.NewReadyShellForTest(t, eng, chatID, dir)
}
