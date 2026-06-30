//go:build windows

package session

// checkForegroundResetLocked is a no-op on Windows: ConPTY exposes no foreground
// process-group, so there is no app→shell edge to detect (§11.1). A SIGKILLed alt-screen app
// may leave stale modes until the next real app output re-establishes them — the same
// limitation the Windows isIdleLocked stub already implies. Caller holds s.mu.
func (s *Session) checkForegroundResetLocked() {}
