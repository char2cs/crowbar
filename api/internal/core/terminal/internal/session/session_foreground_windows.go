//go:build windows

package session

// checkForegroundResetLocked is a no-op on Windows: ConPTY exposes no foreground process
// group, so a killed alt-screen app's modes linger until the next output. Caller holds s.mu.
func (s *Session) checkForegroundResetLocked() {}
