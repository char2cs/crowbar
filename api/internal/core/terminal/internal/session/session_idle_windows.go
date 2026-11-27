//go:build windows

package session

func (s *Session) isIdleLocked() bool { return false }
