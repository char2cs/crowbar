//go:build !windows

package session

func (s *Session) isIdleLocked() bool {
	if s.ptmx == nil || s.cmd == nil || s.cmd.Process == nil {
		return false
	}

	fgPgid, err := ptyForegroundPgid(s.ptmx.Fd())
	if err != nil {
		return false
	}

	shellPgid, err := getpgidFn(s.cmd.Process.Pid)
	if err != nil {
		return false
	}

	return fgPgid == shellPgid
}
