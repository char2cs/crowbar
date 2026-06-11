//go:build windows

package adapter

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(
	file *os.File,
) error {
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrStateDirLocked
	}
	return err
}

func unlockFile(
	file *os.File,
) error {
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
}
