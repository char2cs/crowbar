//go:build !windows

package session

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

func (s *Session) latchForeground() {
	if pgid, err := getpgidFn(s.cmd.Process.Pid); err == nil {
		s.lastForegroundPgid = pgid
	}
}
