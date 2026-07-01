//go:build !windows

package session

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// ptyForegroundPgid reads the PTY master's foreground process-group id via the TIOCGPGRP
// ioctl. It is a package-level var ONLY so a unit test can substitute a probe that returns
// an error, driving the otherwise-unreachable ioctl-failure branch in isIdleLocked and
// checkForegroundResetLocked — a valid live PTY's TIOCGPGRP never fails, so no real input
// reaches those guards. Production never reassigns it.
var ptyForegroundPgid = func(fd uintptr) (int, error) {
	var fgPgid int32
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		fd,
		uintptr(unix.TIOCGPGRP),
		uintptr(unsafe.Pointer(&fgPgid)),
	); errno != 0 {
		return 0, errno
	}
	return int(fgPgid), nil
}

// getpgidFn wraps unix.Getpgid. It is a package-level var ONLY so a unit test can force the
// getpgid-failure branch (the shell's process having vanished between the ioctl and the
// getpgid) which no real, still-live session can reproduce deterministically. Production
// never reassigns it.
var getpgidFn = unix.Getpgid
