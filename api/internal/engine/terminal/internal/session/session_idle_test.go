//go:build !windows

package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsIdle_unix verifies IsIdle using a real PTY:
//   - After the shell settles at its prompt (no foreground child), IsIdle is true.
//   - While a foreground sleep child is running, IsIdle is false.
//
// Determinism strategy:
//
//  1. Idle: attach to the shell output and drain until we receive at least one
//     frame (confirming the shell has started). Then assert IsIdle true. The
//     shell at its prompt IS the foreground process group, so TIOCGPGRP ==
//     shell pgid.
//
//  2. Active: write a long-running `sleep 9999` command, then poll IsIdle
//     with a short interval until it returns false or we time out. The sleep
//     process takes over the foreground process group, so TIOCGPGRP ≠ shell
//     pgid.
func TestIsIdle_unix(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-idle", "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Wait for the first output frame so we know the shell has initialised.
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		// No output yet; that is also fine — the shell may not print a prompt.
		// Fall through to the timed settle below.
	}

	// Extra settle: give the shell time to return to its read loop after
	// any rc-file sourcing completes. 150 ms is generous for /bin/sh.
	time.Sleep(150 * time.Millisecond)

	// --- Idle assertion -------------------------------------------------
	// Poll with retries so that a brief startup race doesn't flake.
	idleTrue := false
	for i := 0; i < 20; i++ {
		if s.IsIdle() {
			idleTrue = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.True(t, idleTrue, "shell at prompt must report IsIdle=true")

	// --- Active assertion -----------------------------------------------
	// Spawn a long-running foreground sleep command.
	require.NoError(t, s.Write([]byte("sleep 9999\n")))

	// Poll until IsIdle flips to false (or time out).
	idleFalse := false
	for i := 0; i < 40; i++ {
		time.Sleep(50 * time.Millisecond)
		if !s.IsIdle() {
			idleFalse = true
			break
		}
	}
	assert.True(t, idleFalse, "shell with foreground sleep child must report IsIdle=false")
}
