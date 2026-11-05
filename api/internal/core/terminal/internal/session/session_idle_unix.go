//go:build !windows

package session

// isIdleLocked reports whether the shell is idle at its prompt: the PTY's foreground
// process group is the shell's own. False on any ioctl/getpgid error or a dead session.
// Caller must hold s.mu.
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
