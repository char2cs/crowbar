//go:build windows

package session

func (s *Session) checkForegroundResetLocked() {}

func (s *Session) latchForeground() {}
