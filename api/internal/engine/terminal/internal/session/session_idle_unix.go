//go:build !windows

package session

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// isIdleLocked reports whether the shell has no foreground child process.
// It uses TIOCGPGRP to get the foreground process group of the PTY and
// compares it to the shell's own process group. If they match, the shell is
// the foreground process — i.e. it is idle (waiting at its prompt).
//
// Caller must hold s.mu.
// Returns false on any ioctl error or if the session is not live.
func (s *Session) isIdleLocked() bool {
	if s.ptmx == nil || s.cmd == nil || s.cmd.Process == nil {
		return false
	}

	// TIOCGPGRP reads the foreground process group ID from the PTY.
	var fgPgid int32
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		s.ptmx.Fd(),
		uintptr(unix.TIOCGPGRP),
		uintptr(unsafe.Pointer(&fgPgid)),
	); errno != 0 {
		return false
	}

	// The shell's process group — typically equal to the shell's PID
	// because it creates its own process group on startup.
	shellPgid, err := unix.Getpgid(s.cmd.Process.Pid)
	if err != nil {
		return false
	}

	return int(fgPgid) == shellPgid
}
