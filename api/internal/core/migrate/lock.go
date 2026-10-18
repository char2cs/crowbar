package migrate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// ErrHomeLocked is returned when another process already holds the home lock.
var ErrHomeLocked = errors.New("migrate: another crowbar instance is using this home")

// homeLock is an OS-level exclusive lock on the home's lock file. The OS frees
// it when the process exits, so a crashed daemon never leaves a stale lock.
type homeLock struct {
	file *os.File
}

// createHome makes the owner-only home directory. Without it there is nothing
// to hold or to report on.
func createHome(
	h home.Home,
) error {
	if err := os.MkdirAll(h.Root(), 0o700); err != nil {
		return fmt.Errorf("migrate: create home: %w", err)
	}
	return nil
}

// lockHome takes the home lock without blocking, creating state/ and the lock
// file when missing. A held lock yields an error wrapping ErrHomeLocked.
func lockHome(
	h home.Home,
) (*homeLock, error) {
	if err := os.MkdirAll(h.State(), 0o750); err != nil {
		return nil, fmt.Errorf("migrate: create state directory: %w", err)
	}
	return acquireLock(h, os.O_CREATE|os.O_RDWR)
}

// Close releases the lock and the lock file. It is safe to call twice.
func (l *homeLock) Close() error {
	if l.file == nil {
		return nil
	}
	err := errors.Join(unlockFile(l.file), l.file.Close())
	l.file = nil
	if err != nil {
		return fmt.Errorf("migrate: release home lock: %w", err)
	}
	return nil
}

// probeLock reports whether another process holds the home lock; an absent or
// unopenable lock file has no provable holder and is never created. It takes
// the lock for an instant, so a daemon booting right then may see ErrHomeLocked.
func probeLock(
	h home.Home,
) bool {
	lock, err := acquireLock(h, os.O_RDWR)
	if errors.Is(err, ErrHomeLocked) {
		return true
	}
	if err != nil {
		return false
	}
	if err := lock.Close(); err != nil {
		slog.Warn("migrate: probe home lock", "home", h.Root(), "err", err)
	}
	return false
}

func acquireLock(
	h home.Home,
	flag int,
) (*homeLock, error) {
	path := h.LockFile()
	file, err := os.OpenFile(path, flag, 0o600) //nolint:gosec // G304: path is <home>/state/.lock, not user input
	if err != nil {
		return nil, fmt.Errorf("migrate: open lock file %q: %w", path, err)
	}
	if err := lockFile(file); err != nil {
		return nil, errors.Join(lockFault(h, err), file.Close())
	}
	return &homeLock{file: file}, nil
}

func lockFault(
	h home.Home,
	err error,
) error {
	if errors.Is(err, ErrHomeLocked) {
		return fmt.Errorf("%w: %s", ErrHomeLocked, h.Root())
	}
	return fmt.Errorf("migrate: lock home %s: %w", h.Root(), err)
}
