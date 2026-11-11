//go:build !windows

package session

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// ptyForegroundPgid reads the PTY master's foreground process group via TIOCGPGRP. It is
// a var only so a test can drive the ioctl-failure branch a live PTY never hits.
// Production never reassigns it.
var ptyForegroundPgid = func(fd uintptr) (int, error) {
	var fgPgid int32
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL, //nolint:staticcheck // SA1019: the direct TIOCGPGRP ioctl is intentional; x/sys's blanket "use libSystem wrappers" deprecation has no drop-in for the pgid-via-pointer probe.
		fd,
		uintptr(unix.TIOCGPGRP),
		uintptr(unsafe.Pointer(&fgPgid)), //nolint:gosec // G103: unsafe.Pointer is required to hand the int32 result buffer to the TIOCGPGRP ioctl.
	); errno != 0 {
		return 0, errno
	}
	return int(fgPgid), nil
}

// getpgidFn wraps unix.Getpgid; it is a var only so a test can force the getpgid
// failure no live session reproduces. Production never reassigns it.
var getpgidFn = unix.Getpgid
