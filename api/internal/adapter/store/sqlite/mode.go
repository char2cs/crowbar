package sqlite

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func OwnerOnly(
	path string,
) error {
	if path == "" || path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o600) //nolint:gosec // G304: path is a database file of the daemon's own home
	switch {
	case errors.Is(err, fs.ErrPermission):
		return nil
	case err != nil:
		return fmt.Errorf("sqlite: create %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sqlite: create %s: %w", path, err)
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := narrow(p); err != nil {
			return err
		}
	}
	return nil
}

func narrow(
	path string,
) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: stat %s: %w", path, err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		if err := os.Chmod(path, perm&^0o077); err != nil {
			return fmt.Errorf("sqlite: restrict %s: %w", path, err)
		}
	}
	return nil
}
