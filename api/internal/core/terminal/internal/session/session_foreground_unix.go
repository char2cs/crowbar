//go:build !windows

package session

// checkForegroundResetLocked fires the model's OnForegroundReset once when the PTY's
// foreground group returns to the shell's (a foreground app exited, even by SIGKILL), so
// its stale alt-screen/mouse modes never leak into the prompt. Caller must hold s.mu.
func (s *Session) checkForegroundResetLocked() {
	if s.ptmx == nil || s.cmd == nil || s.cmd.Process == nil || s.model == nil {
		return
	}

	fg, err := ptyForegroundPgid(s.ptmx.Fd())
	if err != nil {
		return
	}

	shellPgid, err := getpgidFn(s.cmd.Process.Pid)
	if err != nil {
		return
	}

	if fg == shellPgid && s.lastForegroundPgid != shellPgid {
		s.mutateModelLocked(s.model.OnForegroundReset)
		s.dirty = true
		s.lastBlob = nil
	}
	s.lastForegroundPgid = fg
}
