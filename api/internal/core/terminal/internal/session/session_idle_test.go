//go:build !windows

package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	s, err := New(t.Context(), "sid-idle", "/bin/sh", dir, "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	waitPrompt(t, s)
	assert.True(t, s.IsIdle(), "shell at prompt must report IsIdle=true")

	startForeground(t, s)
	assert.False(t, s.IsIdle(), "shell with a foreground child must report IsIdle=false")
}
