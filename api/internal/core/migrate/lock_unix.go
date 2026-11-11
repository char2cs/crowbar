//go:build !windows

package migrate

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
		return ErrHomeLocked
	}
	return err
}

func unlockFile(
	file *os.File,
) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
