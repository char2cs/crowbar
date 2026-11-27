package migrate

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

var ErrHomeLocked = errors.New("migrate: another crowbar instance is using this home")

type homeLock struct {
	file *os.File
}

func createHome(
	h home.Home,
) error {
	if err := os.MkdirAll(h.Root(), 0o700); err != nil {
		return fmt.Errorf("migrate: create home: %w", err)
	}
	return nil
}

func lockHome(
	h home.Home,
) (*homeLock, error) {
	if err := os.MkdirAll(h.State(), 0o700); err != nil {
		return nil, fmt.Errorf("migrate: create state directory: %w", err)
	}
	return acquireLock(h, os.O_CREATE|os.O_RDWR)
}

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
