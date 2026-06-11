//go:build !windows

package adapter

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(
	file *os.File,
) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrStateDirLocked
	}
	return err
}

func unlockFile(
	file *os.File,
) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
