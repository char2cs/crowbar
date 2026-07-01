//go:build !windows

package session

// isIdleLocked reports whether the shell is idle (no foreground child process).
// It reads the PTY's foreground process group (TIOCGPGRP, via the ptyForegroundPgid
// seam) and compares it to the shell's own process group. If they match, the shell is
// the foreground process — i.e. it is idle (waiting at its prompt).
//
// Caller must hold s.mu.
// Returns false on any ioctl/getpgid error or if the session is not live.
func (s *Session) isIdleLocked() bool {
	if s.ptmx == nil || s.cmd == nil || s.cmd.Process == nil {
		return false
	}

	fgPgid, err := ptyForegroundPgid(s.ptmx.Fd())
	if err != nil {
		return false
	}

	// The shell's process group — typically equal to the shell's PID
	// because it creates its own process group on startup.
	shellPgid, err := getpgidFn(s.cmd.Process.Pid)
	if err != nil {
		return false
	}

	return fgPgid == shellPgid
}
