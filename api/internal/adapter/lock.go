package adapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrStateDirLocked is returned when another Crowbar instance already holds the
// exclusive lock on the state directory.
var ErrStateDirLocked = errors.New("state directory locked")

const lockFileName = ".lock"

// instanceLock is an OS-level exclusive lock on a state directory's lockfile. It
// guards against two daemons sharing the same on-disk SQLite databases. The lock
// is released on Unlock and is also freed by the OS when the process exits, so a
// crashed daemon never leaves a stale lock behind.
type instanceLock struct {
	file *os.File
}

// acquireStateLock takes a non-blocking exclusive lock on <stateDir>/.lock. The
// state directory must already exist. If another instance holds the lock it
// returns an error wrapping ErrStateDirLocked.
func acquireStateLock(
	stateDir string,
) (*instanceLock, error) {
	lockPath := filepath.Join(stateDir, lockFileName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("adapter: open lockfile %q: %w", lockPath, err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrStateDirLocked) {
			return nil, fmt.Errorf(
				"another crowbar instance is already using %s (%w)",
				stateDir,
				ErrStateDirLocked,
			)
		}
		return nil, fmt.Errorf("adapter: lock state directory %q: %w", stateDir, err)
	}
	return &instanceLock{file: file}, nil
}

// Close releases the OS lock and closes the underlying lockfile descriptor.
func (l *instanceLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
