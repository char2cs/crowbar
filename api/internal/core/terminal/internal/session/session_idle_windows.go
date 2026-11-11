//go:build windows

package session

// isIdleLocked is a stub on Windows, which has no TIOCGPGRP. Caller must hold s.mu.
func (s *Session) isIdleLocked() bool { return false }
