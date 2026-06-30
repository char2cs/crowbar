//go:build !windows

package session

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// checkForegroundResetLocked samples the PTY's foreground process group and, on the edge
// where it returns to the shell's own group (a foreground app exited, by any means including
// SIGKILL), fires the model's OnForegroundReset exactly once so a later Serialize cannot
// leak the dead app's stale alt-screen/mouse modes into the idle prompt (§11.1). It latches
// lastForegroundPgid so repeated idle samples do not re-fire. The teardown runs through the
// panic-isolated mutateModelLocked. Caller must hold s.mu.
func (s *Session) checkForegroundResetLocked() {
	if s.ptmx == nil || s.cmd == nil || s.cmd.Process == nil || s.model == nil {
		return
	}

	var fgPgid int32
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		s.ptmx.Fd(),
		uintptr(unix.TIOCGPGRP),
		uintptr(unsafe.Pointer(&fgPgid)),
	); errno != 0 {
		return
	}

	shellPgid, err := unix.Getpgid(s.cmd.Process.Pid)
	if err != nil {
		return
	}

	fg := int(fgPgid)
	if fg == shellPgid && s.lastForegroundPgid != shellPgid {
		s.mutateModelLocked(s.model.OnForegroundReset)
		s.dirty = true
		s.lastBlob = nil
	}
	s.lastForegroundPgid = fg
}
