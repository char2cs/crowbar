//go:build !windows

package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsIdle_unix verifies IsIdle against a real PTY, at both of its edges:
//
//   - At its prompt the shell IS the terminal's foreground process group, so IsIdle is true.
//   - With a foreground child running, the child owns the terminal, so IsIdle is false.
//
// Determinism: neither edge is waited out on a clock, because neither has to be. Both are
// OBSERVABLE the moment they happen, and both observations are made through the shell's own
// output:
//
//  1. Idle — waitPrompt blocks until the pinned PS1 is on screen. The prompt is the last thing
//     the shell writes before blocking on read(2), and it writes it as the foreground group, so
//     the instant it is visible IsIdle is already true. Asserted directly; the old
//     assert.Eventually poll would have masked a real "prompt shown while not yet foreground"
//     bug by simply retrying until it went away.
//
//  2. Active — startForeground runs the child as `( announce; exec sleep 9999 )`. Job control
//     puts that job in its own process group and makes it the terminal's FOREGROUND group
//     BEFORE the job runs, and exec preserves the pid/pgroup for the sleep. So the child's very
//     first output byte proves TIOCGPGRP has already flipped away from the shell — IsIdle is
//     false with zero delay, which startForeground asserts rather than polls for.
func TestIsIdle_unix(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-idle", "/bin/sh", dir, "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	// --- Idle assertion -------------------------------------------------
	waitPrompt(t, s)
	assert.True(t, s.IsIdle(), "shell at prompt must report IsIdle=true")

	// --- Active assertion -----------------------------------------------
	// startForeground blocks on the child's announcement and asserts !IsIdle() there.
	startForeground(t, s)
	assert.False(t, s.IsIdle(), "shell with a foreground child must report IsIdle=false")
}
