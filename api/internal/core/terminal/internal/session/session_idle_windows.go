//go:build windows

package session

// isIdleLocked is a stub on Windows. PTY idle detection via TIOCGPGRP is not
// available on this platform.
//
// Caller must hold s.mu.
func (s *Session) isIdleLocked() bool { return false }
